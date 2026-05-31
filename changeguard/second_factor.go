package changeguard

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	goodhttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/log"
	commonsms "github.com/goodbye-jack/go-common/notify/sms"
	"github.com/goodbye-jack/go-common/orm"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	secondFactorRequiredCode = "CHANGEGUARD_SECOND_FACTOR_REQUIRED"
	secondFactorRejectedCode = "CHANGEGUARD_SECOND_FACTOR_REJECTED"
	secondFactorWaitPath     = "/api/v1/system/changeguard/second-factor/result"
)

type secondFactorService struct {
	cfg         SecondFactorSMSConfig
	spec        AppSpec
	serviceName string
	sender      *commonsms.Sender
}

type secondFactorChallenge struct {
	ChallengeID         string            `json:"challenge_id"`
	ServiceName         string            `json:"service_name"`
	ScenarioKey         string            `json:"scenario_key"`
	ResourceKey         string            `json:"resource_key"`
	Action              string            `json:"action"`
	PrincipalUserID     string            `json:"principal_user_id"`
	PrincipalTenantCode string            `json:"principal_tenant_code"`
	Phone               string            `json:"phone"`
	MaskedPhone         string            `json:"masked_phone"`
	RequestDigest       string            `json:"request_digest"`
	SMSCode             string            `json:"sms_code"`
	ReplyToken          string            `json:"reply_token"`
	ApprovalStatus      string            `json:"approval_status"`
	VerifyAttempts      int               `json:"verify_attempts"`
	Verified            bool              `json:"verified"`
	VerifiedAtUnix      int64             `json:"verified_at_unix"`
	ExpiresAtUnix       int64             `json:"expires_at_unix"`
	LastSentAtUnix      int64             `json:"last_sent_at_unix"`
	ConsumedAtUnix      int64             `json:"consumed_at_unix"`
	Metadata            map[string]string `json:"metadata"`
}

type secondFactorReplyPayload struct {
	Mobile  string `form:"mobile" json:"mobile"`
	Message string `form:"message" json:"message"`
}

type secondFactorWaitResult struct {
	Status          string `json:"status"`
	ChallengeID     string `json:"challenge_id"`
	ExpireInSeconds int64  `json:"expire_in_seconds"`
	ExpiresAtUnix   int64  `json:"expires_at_unix"`
	Message         string `json:"message"`
}

func newSecondFactorService(spec AppSpec, serviceName string, cfg SecondFactorSMSConfig) (*secondFactorService, bool) {
	cfg.Normalize()
	if !cfg.Enabled {
		return nil, false
	}
	if orm.Redis == nil {
		log.Warnf("changeguard second factor disabled at runtime: redis unavailable")
		return nil, false
	}
	smsCfg, err := NewConfigPrefixSMSResolver(cfg.ConfigPrefix).Resolve(context.Background())
	if err != nil {
		log.Warnf("changeguard second factor sms config invalid: %v", err)
		return nil, false
	}
	sender, err := commonsms.NewSender(smsCfg)
	if err != nil {
		log.Warnf("changeguard second factor sender init failed: %v", err)
		return nil, false
	}
	return &secondFactorService{
		cfg:         cfg,
		spec:        spec,
		serviceName: serviceName,
		sender:      sender,
	}, true
}

func (c *SecondFactorSMSConfig) Normalize() {
	if c == nil {
		return
	}
	c.ConfigPrefix = firstNonBlank(c.ConfigPrefix, "notifications.sms")
	c.Template = firstNonBlank(c.Template, "【关键资源保护】您正在执行{{resource_name}}{{action_name}}操作，摘要：{{summary_text}}。短信验证码：{{code}}，{{ttl_minutes}}分钟内有效。")
	c.ReplyTemplate = firstNonBlank(c.ReplyTemplate, "【关键资源保护】您正在执行{{resource_name}}{{action_name}}操作，摘要：{{summary_text}}。同意请回复 1#{{reply_token}}，拒绝请回复 0#{{reply_token}}。")
	c.ChallengeHeader = firstNonBlank(c.ChallengeHeader, "X-ChangeGuard-Challenge-Id")
	c.CodeHeader = firstNonBlank(c.CodeHeader, "X-ChangeGuard-Verify-Code")
	c.ReplyCallbackPath = firstNonBlank(c.ReplyCallbackPath, "/internal/changeguard/second-factor/sms-reply")
	c.ReplyApprovePrefix = firstNonBlank(c.ReplyApprovePrefix, "1#")
	c.ReplyRejectPrefix = firstNonBlank(c.ReplyRejectPrefix, "0#")
	c.UserIDField = firstNonBlank(c.UserIDField, "id")
	c.PhoneField = firstNonBlank(c.PhoneField, "phone")
	c.TenantField = firstNonBlank(c.TenantField, "tenant_code")
	c.UserTypeField = firstNonBlank(c.UserTypeField, "user_type")
	c.StatusField = firstNonBlank(c.StatusField, "status")
	c.RequiredUserType = firstNonBlank(c.RequiredUserType, "admin")
	c.RequiredStatus = firstNonBlank(c.RequiredStatus, "normal")
	if c.CodeTTL == "" {
		c.CodeTTL = "5m"
	}
	if c.ResendInterval == "" {
		c.ResendInterval = "60s"
	}
	if c.VerifiedTTL == "" {
		c.VerifiedTTL = "10m"
	}
	if c.MaxVerifyAttempts <= 0 {
		c.MaxVerifyAttempts = 5
	}
	if c.ReplyTokenSize <= 0 {
		c.ReplyTokenSize = 2
	}
}

func (s *secondFactorService) shouldProtect(policy PolicyProfile, action string) bool {
	if s == nil || !policy.RequireSecondFactor {
		return false
	}
	if strings.TrimSpace(policy.SecondFactorMode) == "" {
		return false
	}
	if len(policy.SecondFactorOnActions) == 0 {
		return true
	}
	return containsString(policy.SecondFactorOnActions, action)
}

func (s *secondFactorService) ensureVerified(c *gin.Context, session *Session) SecondFactorResult {
	if s == nil || c == nil || session == nil {
		return SecondFactorResult{Allowed: true}
	}
	challenge, err := s.loadMatchingChallenge(c, session)
	if err != nil {
		log.Warnf("changeguard second factor challenge load failed: %v", err)
		return s.sendOrRequire(c, session, nil)
	}
	if challenge == nil {
		return s.sendOrRequire(c, session, nil)
	}
	if s.isConsumed(challenge) || s.isExpired(challenge) {
		return s.sendOrRequire(c, session, challenge)
	}
	if s.isReplyRejected(challenge) {
		return SecondFactorResult{
			Responded:       true,
			HTTPStatus:      http.StatusForbidden,
			ResponseCode:    secondFactorRejectedCode,
			ResponseMessage: "已拒绝本次关键操作",
		}
	}
	if s.isAllowedByMode(c, session, challenge) {
		session.Store["second_factor_challenge_id"] = challenge.ChallengeID
		return SecondFactorResult{Allowed: true}
	}
	return s.sendOrRequire(c, session, challenge)
}

func (s *secondFactorService) sendOrRequire(c *gin.Context, session *Session, existing *secondFactorChallenge) SecondFactorResult {
	challenge, err := s.createOrRefreshChallenge(c, session, existing)
	if err != nil {
		log.Warnf("changeguard second factor challenge create failed: %v", err)
		return SecondFactorResult{
			Responded:       true,
			HTTPStatus:      http.StatusForbidden,
			ResponseCode:    secondFactorRequiredCode,
			ResponseMessage: "当前操作需要短信二次验证，但验证码发送失败",
		}
	}
	data := map[string]any{
		"required":             true,
		"mode":                 session.Policy.SecondFactorMode,
		"challenge_id":         challenge.ChallengeID,
		"expire_in_seconds":    s.remainingSeconds(challenge.ExpiresAtUnix),
		"expires_at_unix":      challenge.ExpiresAtUnix,
		"resend_after_seconds": s.resendAfterSeconds(challenge.LastSentAtUnix),
		"masked_phone":         challenge.MaskedPhone,
		"resource_name":        chooseNonEmpty(session.Resource.Name, session.Resource.Key),
		"action":               session.Action,
		"wait_result_path":     secondFactorWaitPath,
	}
	if summaryText := strings.TrimSpace(challenge.Metadata["summary_text"]); summaryText != "" {
		data["summary_text"] = summaryText
	}
	if challenge.ReplyToken != "" {
		data["reply_token_hint"] = challenge.ReplyToken
	}
	return SecondFactorResult{
		Responded:       true,
		HTTPStatus:      http.StatusPreconditionRequired,
		ResponseCode:    secondFactorRequiredCode,
		ResponseMessage: "当前操作需要短信二次验证",
		ResponseData:    data,
	}
}

func (s *secondFactorService) createOrRefreshChallenge(c *gin.Context, session *Session, existing *secondFactorChallenge) (*secondFactorChallenge, error) {
	phone, err := s.resolveOperatorPhone(c, session)
	if err != nil {
		return nil, err
	}
	mode := normalizeSecondFactorMode(session.Policy.SecondFactorMode)
	if mode == "" {
		return nil, fmt.Errorf("second factor mode is empty")
	}
	now := time.Now()
	codeTTL := s.mustDuration(s.cfg.CodeTTL, 5*time.Minute)
	requestDigest := buildSecondFactorRequestDigest(session)
	challenge := existing
	if challenge == nil || challenge.RequestDigest != requestDigest || s.isExpired(challenge) || s.isConsumed(challenge) || s.isReplyRejected(challenge) {
		code, err := generateNumericCode(6)
		if err != nil {
			return nil, err
		}
		replyToken, err := generateReplyToken(s.cfg.ReplyTokenSize)
		if err != nil {
			return nil, err
		}
		summaryText := buildSecondFactorSummary(session)
		challenge = &secondFactorChallenge{
			ChallengeID:         uuid.NewString(),
			ServiceName:         s.serviceName,
			ScenarioKey:         firstNonBlank(session.Binding.Metadata["scenario_key"], session.Resource.Key),
			ResourceKey:         session.Resource.Key,
			Action:              session.Action,
			PrincipalUserID:     session.Principal.UserID,
			PrincipalTenantCode: session.Principal.TenantCode,
			Phone:               phone,
			MaskedPhone:         maskPhone(phone),
			RequestDigest:       requestDigest,
			SMSCode:             code,
			ReplyToken:          replyToken,
			ApprovalStatus:      SecondFactorStatusPending,
			ExpiresAtUnix:       now.Add(codeTTL).Unix(),
			LastSentAtUnix:      0,
			Metadata: map[string]string{
				"mode":          mode,
				"resource_name": chooseNonEmpty(session.Resource.Name, session.Resource.Key),
				"action_name":   actionDisplayName(session.Action),
				"summary_text":  summaryText,
			},
		}
	} else {
		challenge.ExpiresAtUnix = now.Add(codeTTL).Unix()
		challenge.Metadata["mode"] = mode
		challenge.Metadata["resource_name"] = chooseNonEmpty(session.Resource.Name, session.Resource.Key)
		challenge.Metadata["action_name"] = actionDisplayName(session.Action)
		challenge.Metadata["summary_text"] = buildSecondFactorSummary(session)
	}
	if !s.canResend(challenge.LastSentAtUnix) {
		return challenge, s.saveChallenge(context.Background(), challenge)
	}
	if err := s.sendChallenge(context.Background(), session, challenge, mode); err != nil {
		return nil, err
	}
	challenge.LastSentAtUnix = now.Unix()
	if err := s.saveChallenge(context.Background(), challenge); err != nil {
		return nil, err
	}
	return challenge, nil
}

func (s *secondFactorService) sendChallenge(ctx context.Context, session *Session, challenge *secondFactorChallenge, mode string) error {
	if s == nil || challenge == nil || s.sender == nil {
		return fmt.Errorf("second factor sender unavailable")
	}
	var template string
	summaryText := firstNonBlank(challenge.Metadata["summary_text"], buildSecondFactorSummary(session))
	variables := map[string]string{
		"code":          challenge.SMSCode,
		"resource_name": chooseNonEmpty(session.Resource.Name, session.Resource.Key),
		"action_name":   actionDisplayName(session.Action),
		"ttl_minutes":   strconv.FormatInt(int64(s.mustDuration(s.cfg.CodeTTL, 5*time.Minute).Minutes()), 10),
		"operator_name": chooseNonEmpty(session.Principal.UserName, session.Principal.UserAccount, session.Principal.UserID, "未知操作人"),
		"reply_token":   challenge.ReplyToken,
		"summary_text":  summaryText,
	}
	switch mode {
	case SecondFactorModeSMSReply:
		template = s.cfg.ReplyTemplate
	default:
		template = s.cfg.Template
	}
	message := commonsms.Message{
		Phone:     challenge.Phone,
		Template:  template,
		Variables: variables,
	}
	finalContent := s.sender.PreviewContent(message)
	log.Infof("changeguard second factor sms sending, challenge_id=%s, phone=%s, mode=%s, content=%s", challenge.ChallengeID, challenge.Phone, mode, finalContent)
	result, err := s.sender.Send(ctx, message)
	if err != nil {
		log.Warnf("changeguard second factor sms failed, challenge_id=%s, phone=%s, err=%v", challenge.ChallengeID, challenge.Phone, err)
		return err
	}
	if result != nil {
		log.Infof("changeguard second factor sms success, challenge_id=%s, phone=%s, provider=%s, request_id=%s", challenge.ChallengeID, challenge.Phone, result.Provider, result.ProviderRequestID)
	}
	return nil
}

func (s *secondFactorService) isAllowedByMode(c *gin.Context, session *Session, challenge *secondFactorChallenge) bool {
	mode := normalizeSecondFactorMode(session.Policy.SecondFactorMode)
	switch mode {
	case SecondFactorModeSMSReply:
		return challenge.ApprovalStatus == SecondFactorStatusApproved && !s.isExpired(challenge)
	case SecondFactorModeSMSCodeOrReply:
		if challenge.ApprovalStatus == SecondFactorStatusApproved && !s.isExpired(challenge) {
			return true
		}
		return s.tryVerifyCode(c, challenge)
	default:
		return s.tryVerifyCode(c, challenge)
	}
}

func (s *secondFactorService) tryVerifyCode(c *gin.Context, challenge *secondFactorChallenge) bool {
	if c == nil || challenge == nil {
		return false
	}
	challengeID := strings.TrimSpace(c.GetHeader(s.cfg.ChallengeHeader))
	code := strings.TrimSpace(c.GetHeader(s.cfg.CodeHeader))
	if challengeID == "" || code == "" || challengeID != challenge.ChallengeID {
		return false
	}
	if challenge.VerifyAttempts >= s.cfg.MaxVerifyAttempts {
		return false
	}
	if challenge.SMSCode != code {
		challenge.VerifyAttempts++
		_ = s.saveChallenge(context.Background(), challenge)
		return false
	}
	challenge.VerifyAttempts = 0
	challenge.Verified = true
	challenge.VerifiedAtUnix = time.Now().Unix()
	challenge.ApprovalStatus = SecondFactorStatusVerified
	_ = s.saveChallenge(context.Background(), challenge)
	return true
}

func registerSecondFactorReplyRoute(server interface {
	RouteWithPolicy(string, string, []string, goodhttp.AuthPolicy, gin.HandlerFunc)
}, service *secondFactorService) {
	if server == nil || service == nil {
		return
	}
	server.RouteWithPolicy(service.cfg.ReplyCallbackPath, "changeguard 短信上行回复回调", []string{"GET"}, goodhttp.Public(), service.handleReplyCallback)
	server.RouteWithPolicy(secondFactorWaitPath, "changeguard 短信二次确认结果", []string{"GET"}, goodhttp.Admin(goodhttp.CookieOnly()), service.handleWaitResult)
}

func (s *secondFactorService) handleReplyCallback(c *gin.Context) {
	if c == nil {
		return
	}
	payload := secondFactorReplyPayload{
		Mobile:  strings.TrimSpace(c.Query("mobile")),
		Message: strings.TrimSpace(c.Query("message")),
	}
	if payload.Mobile == "" || payload.Message == "" {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	if err := s.processReply(context.Background(), payload.Mobile, payload.Message); err != nil {
		log.Warnf("changeguard second factor reply process failed: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *secondFactorService) handleWaitResult(c *gin.Context) {
	if c == nil {
		return
	}
	challengeID := strings.TrimSpace(c.Query("challenge_id"))
	if challengeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "challenge_id不能为空",
		})
		return
	}
	principal := ResolvePrincipal(c)
	if strings.TrimSpace(principal.UserID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "登录已失效，请重新登录",
		})
		return
	}
	result, statusCode, err := s.waitChallengeResult(c.Request.Context(), principal, challengeID)
	if err != nil {
		c.JSON(statusCode, gin.H{
			"code":    statusCode,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "成功",
		"success": true,
		"data":    result,
	})
}

func (s *secondFactorService) processReply(ctx context.Context, mobile, message string) error {
	challenge, action, err := s.findChallengeByReply(mobile, message)
	if err != nil || challenge == nil {
		return err
	}
	switch action {
	case "approve":
		challenge.ApprovalStatus = SecondFactorStatusApproved
		challenge.Verified = true
		challenge.VerifiedAtUnix = time.Now().Unix()
	case "reject":
		challenge.ApprovalStatus = SecondFactorStatusRejected
	default:
		return nil
	}
	return s.saveChallenge(ctx, challenge)
}

func (s *secondFactorService) waitChallengeResult(ctx context.Context, principal Principal, challengeID string) (secondFactorWaitResult, int, error) {
	challenge, err := s.loadChallenge(context.Background(), challengeID)
	if err != nil || challenge == nil {
		return secondFactorWaitResult{}, http.StatusNotFound, fmt.Errorf("二次确认不存在或已失效")
	}
	if strings.TrimSpace(challenge.PrincipalUserID) != strings.TrimSpace(principal.UserID) ||
		strings.TrimSpace(challenge.PrincipalTenantCode) != strings.TrimSpace(principal.TenantCode) {
		return secondFactorWaitResult{}, http.StatusForbidden, fmt.Errorf("无权查看当前二次确认状态")
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		challenge, err = s.loadChallenge(context.Background(), challengeID)
		if err != nil || challenge == nil {
			return secondFactorWaitResult{}, http.StatusNotFound, fmt.Errorf("二次确认不存在或已失效")
		}
		if result, done := s.inspectChallengeWaitResult(challenge); done {
			return result, http.StatusOK, nil
		}
		select {
		case <-ctx.Done():
			return secondFactorWaitResult{}, http.StatusRequestTimeout, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *secondFactorService) inspectChallengeWaitResult(challenge *secondFactorChallenge) (secondFactorWaitResult, bool) {
	if challenge == nil {
		return secondFactorWaitResult{}, true
	}
	if s.isExpired(challenge) {
		return s.buildWaitResult(challenge, "expired", "短信确认已过期，请重新发起操作"), true
	}
	switch challenge.ApprovalStatus {
	case SecondFactorStatusApproved, SecondFactorStatusConsumed:
		return s.buildWaitResult(challenge, "approved", "短信确认已通过"), true
	case SecondFactorStatusRejected:
		return s.buildWaitResult(challenge, "rejected", "当前操作已被短信拒绝"), true
	default:
		return secondFactorWaitResult{}, false
	}
}

func (s *secondFactorService) buildWaitResult(challenge *secondFactorChallenge, status string, message string) secondFactorWaitResult {
	expiresAtUnix := int64(0)
	challengeID := ""
	if challenge != nil {
		expiresAtUnix = challenge.ExpiresAtUnix
		challengeID = challenge.ChallengeID
	}
	return secondFactorWaitResult{
		Status:          status,
		ChallengeID:     challengeID,
		ExpireInSeconds: s.remainingSeconds(expiresAtUnix),
		ExpiresAtUnix:   expiresAtUnix,
		Message:         message,
	}
}

func (s *secondFactorService) findChallengeByReply(mobile, message string) (*secondFactorChallenge, string, error) {
	message = strings.TrimSpace(strings.ToUpper(message))
	approvePrefix := strings.ToUpper(strings.TrimSpace(s.cfg.ReplyApprovePrefix))
	rejectPrefix := strings.ToUpper(strings.TrimSpace(s.cfg.ReplyRejectPrefix))
	action := ""
	token := ""
	switch {
	case strings.HasPrefix(message, approvePrefix):
		action = "approve"
		token = strings.TrimSpace(strings.TrimPrefix(message, approvePrefix))
	case strings.HasPrefix(message, rejectPrefix):
		action = "reject"
		token = strings.TrimSpace(strings.TrimPrefix(message, rejectPrefix))
	default:
		return nil, "", nil
	}
	if token == "" {
		return nil, "", nil
	}
	challenge, err := s.loadChallengeByReplyToken(context.Background(), mobile, token)
	return challenge, action, err
}

func (s *secondFactorService) loadMatchingChallenge(c *gin.Context, session *Session) (*secondFactorChallenge, error) {
	if s == nil || session == nil || orm.Redis == nil {
		return nil, nil
	}
	challengeID := strings.TrimSpace(c.GetHeader(s.cfg.ChallengeHeader))
	requestDigest := buildSecondFactorRequestDigest(session)
	if challengeID != "" {
		challenge, err := s.loadChallenge(context.Background(), challengeID)
		if err != nil || challenge == nil {
			return nil, err
		}
		if challenge.RequestDigest != requestDigest || challenge.PrincipalUserID != session.Principal.UserID || challenge.PrincipalTenantCode != session.Principal.TenantCode {
			return nil, nil
		}
		return challenge, nil
	}
	return s.loadLatestChallenge(context.Background(), session.Principal.UserID, session.Principal.TenantCode, requestDigest)
}

func (s *secondFactorService) loadLatestChallenge(ctx context.Context, userID, tenantCode, requestDigest string) (*secondFactorChallenge, error) {
	if orm.DB != nil {
		record := &SecondFactorChallengeRecord{}
		err := orm.DB.GetDB().WithContext(ctx).
			Model(&SecondFactorChallengeRecord{}).
			Where("principal_user_id = ? AND principal_tenant_code = ? AND request_digest = ?", userID, tenantCode, requestDigest).
			Order("id DESC").
			First(record).Error
		if err == nil {
			return recordToChallenge(record)
		}
	}
	return nil, nil
}

func (s *secondFactorService) loadChallenge(ctx context.Context, challengeID string) (*secondFactorChallenge, error) {
	if orm.Redis != nil {
		if raw, err := orm.Redis.Get(ctx, secondFactorRedisKey(challengeID)); err == nil && strings.TrimSpace(raw) != "" {
			challenge := &secondFactorChallenge{}
			if err := json.Unmarshal([]byte(raw), challenge); err == nil {
				return challenge, nil
			}
		}
	}
	if orm.DB == nil {
		return nil, nil
	}
	record := &SecondFactorChallengeRecord{}
	err := orm.DB.GetDB().WithContext(ctx).Model(&SecondFactorChallengeRecord{}).Where("challenge_id = ?", challengeID).First(record).Error
	if err != nil {
		return nil, nil
	}
	return recordToChallenge(record)
}

func (s *secondFactorService) loadChallengeByReplyToken(ctx context.Context, mobile, token string) (*secondFactorChallenge, error) {
	if orm.DB == nil {
		return nil, nil
	}
	record := &SecondFactorChallengeRecord{}
	err := orm.DB.GetDB().WithContext(ctx).
		Model(&SecondFactorChallengeRecord{}).
		Where("phone = ? AND reply_token = ? AND approval_status = ?", mobile, token, SecondFactorStatusPending).
		Order("id DESC").
		First(record).Error
	if err != nil {
		return nil, nil
	}
	return recordToChallenge(record)
}

func (s *secondFactorService) saveChallenge(ctx context.Context, challenge *secondFactorChallenge) error {
	if challenge == nil {
		return nil
	}
	if orm.Redis != nil {
		payload, _ := json.Marshal(challenge)
		ttl := time.Until(time.Unix(challenge.ExpiresAtUnix, 0))
		if challenge.VerifiedAtUnix > 0 {
			verifiedExpire := time.Unix(challenge.VerifiedAtUnix, 0).Add(s.mustDuration(s.cfg.VerifiedTTL, 10*time.Minute))
			if verifiedExpire.After(time.Unix(challenge.ExpiresAtUnix, 0)) {
				ttl = time.Until(verifiedExpire)
			}
		}
		if ttl <= 0 {
			ttl = time.Minute
		}
		if err := orm.Redis.Set(ctx, secondFactorRedisKey(challenge.ChallengeID), string(payload), ttl); err != nil {
			log.Warnf("changeguard second factor redis save failed, challenge_id=%s, err=%v", challenge.ChallengeID, err)
		}
	}
	if orm.DB == nil {
		return nil
	}
	metadataJSON, _ := json.Marshal(challenge.Metadata)
	record := &SecondFactorChallengeRecord{
		ChallengeID:         challenge.ChallengeID,
		ServiceName:         challenge.ServiceName,
		ScenarioKey:         challenge.ScenarioKey,
		ResourceKey:         challenge.ResourceKey,
		Action:              challenge.Action,
		PrincipalUserID:     challenge.PrincipalUserID,
		PrincipalTenantCode: challenge.PrincipalTenantCode,
		Phone:               challenge.Phone,
		MaskedPhone:         challenge.MaskedPhone,
		RequestDigest:       challenge.RequestDigest,
		SMSCode:             challenge.SMSCode,
		ReplyToken:          challenge.ReplyToken,
		ApprovalStatus:      challenge.ApprovalStatus,
		VerifyAttempts:      challenge.VerifyAttempts,
		Verified:            challenge.Verified,
		VerifiedAtUnix:      challenge.VerifiedAtUnix,
		ExpiresAtUnix:       challenge.ExpiresAtUnix,
		LastSentAtUnix:      challenge.LastSentAtUnix,
		ConsumedAtUnix:      challenge.ConsumedAtUnix,
		MetadataJSON:        string(metadataJSON),
	}
	return orm.DB.GetDB().WithContext(ctx).Where("challenge_id = ?", challenge.ChallengeID).Assign(record).FirstOrCreate(&SecondFactorChallengeRecord{}).Error
}

func (s *secondFactorService) consumeChallenge(ctx context.Context, challengeID string) {
	challenge, err := s.loadChallenge(ctx, challengeID)
	if err != nil || challenge == nil {
		return
	}
	challenge.ApprovalStatus = SecondFactorStatusConsumed
	challenge.ConsumedAtUnix = time.Now().Unix()
	_ = s.saveChallenge(ctx, challenge)
}

func (s *secondFactorService) resolveOperatorPhone(c *gin.Context, session *Session) (string, error) {
	if orm.DB == nil || s.spec.UserModel == nil || session == nil {
		return "", fmt.Errorf("当前操作人手机号不可用")
	}
	lookupFields := compactStrings([]string{
		s.cfg.UserIDField,
		"user_name",
		s.cfg.PhoneField,
		s.cfg.TenantField,
		s.cfg.UserTypeField,
		s.cfg.StatusField,
	})
	buildBaseDB := func() *gorm.DB {
		db := orm.DB.GetDB().WithContext(c.Request.Context()).
			Model(s.spec.UserModel).
			Select(strings.Join(lookupFields, ","))
		if session.Principal.TenantCode != "" {
			db = db.Where(s.cfg.TenantField+" = ?", session.Principal.TenantCode)
		}
		if s.cfg.RequiredUserType != "" {
			db = db.Where(s.cfg.UserTypeField+" = ?", s.cfg.RequiredUserType)
		}
		if s.cfg.RequiredStatus != "" {
			db = db.Where(s.cfg.StatusField+" = ?", s.cfg.RequiredStatus)
		}
		return db
	}

	candidates := s.buildOperatorLookupCandidates(session.Principal)
	for _, candidate := range candidates {
		rows := make([]map[string]any, 0, 1)
		db := buildBaseDB().Where(candidate.field+" = ?", candidate.value)
		if err := db.Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
			continue
		}
		value, ok := findMapValueCaseInsensitive(rows[0], s.cfg.PhoneField)
		if !ok {
			continue
		}
		phone := strings.TrimSpace(fmt.Sprint(value))
		if phone != "" {
			return phone, nil
		}
	}
	log.Warnf("changeguard second factor operator phone lookup miss, tenant=%s, user_id=%s, user_name=%s, user_account=%s, candidates=%v",
		session.Principal.TenantCode, session.Principal.UserID, session.Principal.UserName, session.Principal.UserAccount, candidates)
	return "", fmt.Errorf("当前操作人手机号不可用")
}

type operatorLookupCandidate struct {
	field string
	value string
}

func (s *secondFactorService) buildOperatorLookupCandidates(principal Principal) []operatorLookupCandidate {
	candidates := make([]operatorLookupCandidate, 0, 4)
	appendCandidate := func(field string, values ...string) {
		field = strings.TrimSpace(field)
		if field == "" {
			return
		}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			exists := false
			for _, candidate := range candidates {
				if candidate.field == field && candidate.value == value {
					exists = true
					break
				}
			}
			if !exists {
				candidates = append(candidates, operatorLookupCandidate{field: field, value: value})
			}
		}
	}

	appendCandidate(s.cfg.UserIDField, principal.UserID)
	appendCandidate("user_name", principal.UserName, extractUserNameFromSubject(principal.UserAccount), extractUserNameFromSubject(principal.UserID))
	appendCandidate(s.cfg.PhoneField, principal.UserAccount)
	return candidates
}

func extractUserNameFromSubject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.SplitN(raw, "#", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return raw
}

func (s *secondFactorService) canResend(lastSentAtUnix int64) bool {
	if lastSentAtUnix <= 0 {
		return true
	}
	return time.Since(time.Unix(lastSentAtUnix, 0)) >= s.mustDuration(s.cfg.ResendInterval, time.Minute)
}

func (s *secondFactorService) isExpired(challenge *secondFactorChallenge) bool {
	if challenge == nil {
		return true
	}
	if time.Now().Unix() > challenge.ExpiresAtUnix {
		if challenge.VerifiedAtUnix > 0 {
			return time.Now().After(time.Unix(challenge.VerifiedAtUnix, 0).Add(s.mustDuration(s.cfg.VerifiedTTL, 10*time.Minute)))
		}
		return true
	}
	return false
}

func (s *secondFactorService) isConsumed(challenge *secondFactorChallenge) bool {
	return challenge != nil && challenge.ApprovalStatus == SecondFactorStatusConsumed
}

func (s *secondFactorService) isReplyRejected(challenge *secondFactorChallenge) bool {
	return challenge != nil && challenge.ApprovalStatus == SecondFactorStatusRejected
}

func (s *secondFactorService) remainingSeconds(expiresAtUnix int64) int64 {
	if expiresAtUnix <= 0 {
		return 0
	}
	remaining := expiresAtUnix - time.Now().Unix()
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *secondFactorService) resendAfterSeconds(lastSentAtUnix int64) int64 {
	if lastSentAtUnix <= 0 {
		return 0
	}
	remaining := int64(s.mustDuration(s.cfg.ResendInterval, time.Minute).Seconds()) - time.Now().Unix() + lastSentAtUnix
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *secondFactorService) mustDuration(raw string, fallback time.Duration) time.Duration {
	if duration, ok := parseOptionalDuration(raw); ok {
		return duration
	}
	return fallback
}

func normalizeSecondFactorMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case SecondFactorModeSMSCode, SecondFactorModeSMSReply, SecondFactorModeSMSCodeOrReply:
		return strings.TrimSpace(mode)
	default:
		return SecondFactorModeSMSCode
	}
}

func buildSecondFactorRequestDigest(session *Session) string {
	if session == nil {
		return ""
	}
	body := session.RequestMeta.RawBody
	if len(body) == 0 && session.Context != nil {
		if cached, err := GetCachedRequestBody(session.Context); err == nil {
			body = cached
		}
	}
	raw := strings.Join([]string{
		session.Binding.Metadata["scenario_key"],
		session.Resource.Key,
		session.Action,
		session.RequestMeta.Path,
		session.RequestMeta.Method,
		session.Principal.UserID,
		session.Principal.TenantCode,
		session.RequestMeta.QueryString,
		string(body),
	}, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func buildSecondFactorSummary(session *Session) string {
	if session == nil {
		return ""
	}
	body, err := GetCachedJSONMap(session.Context)
	if err != nil || len(body) == 0 {
		return chooseNonEmpty(session.Resource.Name, session.Resource.Key)
	}
	summary := buildBusinessSummaryText(strings.TrimSpace(session.Binding.Metadata["scenario_key"]), body, 5)
	if summary == "" {
		return chooseNonEmpty(session.Resource.Name, session.Resource.Key)
	}
	return summary
}

func generateNumericCode(length int) (string, error) {
	if length <= 0 {
		length = 6
	}
	const digits = "0123456789"
	return generateRandomStringFromSet(length, digits)
}

func generateReplyToken(length int) (string, error) {
	if length <= 0 {
		length = 2
	}
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	return generateRandomStringFromSet(length, charset)
}

func generateRandomStringFromSet(length int, charset string) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("invalid random string length")
	}
	buffer := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for i := 0; i < length; i++ {
		buffer[i] = charset[int(random[i])%len(charset)]
	}
	return string(buffer), nil
}

func secondFactorRedisKey(challengeID string) string {
	return "changeguard:2fa:challenge:" + strings.TrimSpace(challengeID)
}

func recordToChallenge(record *SecondFactorChallengeRecord) (*secondFactorChallenge, error) {
	if record == nil {
		return nil, nil
	}
	challenge := &secondFactorChallenge{
		ChallengeID:         record.ChallengeID,
		ServiceName:         record.ServiceName,
		ScenarioKey:         record.ScenarioKey,
		ResourceKey:         record.ResourceKey,
		Action:              record.Action,
		PrincipalUserID:     record.PrincipalUserID,
		PrincipalTenantCode: record.PrincipalTenantCode,
		Phone:               record.Phone,
		MaskedPhone:         record.MaskedPhone,
		RequestDigest:       record.RequestDigest,
		SMSCode:             record.SMSCode,
		ReplyToken:          record.ReplyToken,
		ApprovalStatus:      record.ApprovalStatus,
		VerifyAttempts:      record.VerifyAttempts,
		Verified:            record.Verified,
		VerifiedAtUnix:      record.VerifiedAtUnix,
		ExpiresAtUnix:       record.ExpiresAtUnix,
		LastSentAtUnix:      record.LastSentAtUnix,
		ConsumedAtUnix:      record.ConsumedAtUnix,
	}
	if record.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(record.MetadataJSON), &challenge.Metadata)
	}
	return challenge, nil
}

func maskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) < 7 {
		return phone
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}
