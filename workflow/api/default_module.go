package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
	commonhttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/utils"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/directory"
	"github.com/goodbye-jack/go-common/workflow/engine/flowable"
	"github.com/goodbye-jack/go-common/workflow/formref"
	"github.com/goodbye-jack/go-common/workflow/types"
)

const (
	configAPIPrefix     = "workflow.api.prefix"
	configAPISSOEnabled = "workflow.api.sso_enabled"
	configAPIRoles      = "workflow.api.roles"
	configCallbackPath  = "workflow.api.callback_path"
	configCallbackKey   = "workflow.api.callback_secret"
)

var defaultRouteRoles = []string{
	utils.UserAnonymous,
	utils.RoleAdministrator,
	utils.RoleDefault,
	utils.RoleMuseum,
	utils.RoleMuseumOffice,
	utils.RoleAppraisalStation,
}

type DefaultModule struct {
	resolver     workflowcontext.Resolver
	directory    directory.Service
	flowable     flowable.Client
	formref      formref.Service
	routePrefix  string
	routeRoles   []string
	requireSSO   bool
	callbackPath string
	callbackKey  string
}

func NewDefaultModule(resolver workflowcontext.Resolver, directoryService directory.Service, flowableClient flowable.Client) (*DefaultModule, error) {
	if resolver == nil {
		return nil, errors.New("workflow context resolver is required")
	}
	if directoryService == nil {
		return nil, errors.New("workflow directory service is required")
	}
	if flowableClient == nil {
		return nil, errors.New("workflow flowable client is required")
	}
	return &DefaultModule{
		resolver:     resolver,
		directory:    directoryService,
		flowable:     flowableClient,
		routePrefix:  normalizedPrefix(config.GetConfigString(configAPIPrefix)),
		routeRoles:   configuredRoles(config.GetConfigString(configAPIRoles)),
		requireSSO:   configuredBool(config.GetConfigString(configAPISSOEnabled), true),
		callbackPath: configuredCallbackPath(config.GetConfigString(configCallbackPath)),
		callbackKey:  strings.TrimSpace(config.GetConfigString(configCallbackKey)),
	}, nil
}

func NewDefaultModuleFromConfig() (*DefaultModule, error) {
	resolver := workflowcontext.NewDefaultResolver()
	directoryService, err := directory.NewLDAPServiceFromConfig()
	if err != nil {
		return nil, err
	}
	flowableClient, err := flowable.NewRESTClientFromConfig()
	if err != nil {
		return nil, err
	}
	module, err := NewDefaultModule(resolver, directoryService, flowableClient)
	if err != nil {
		return nil, err
	}
	module.formref = formref.NewFlowableService(flowableClient)
	return module, nil
}

func (m *DefaultModule) WithFormRefService(service formref.Service) *DefaultModule {
	if m == nil {
		return nil
	}
	m.formref = service
	return m
}

func (m *DefaultModule) Register(server *commonhttp.HTTPServer) {
	if server == nil {
		return
	}
	server.RouteAPI(m.route("/me"), "workflow current user", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleMe)
	server.RouteAPI(m.route("/me/tasks/todo"), "workflow todo tasks", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleTodo)
	server.RouteAPI(m.route("/me/tasks/done"), "workflow done tasks", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleDone)
	server.RouteAPI(m.route("/me/manager"), "workflow current user manager", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleMyManager)
	server.RouteAPI(m.route("/me/department"), "workflow current user department", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleMyDepartment)
	server.RouteAPI(m.route("/process/start"), "workflow start process", []string{http.MethodPost}, m.routeRoles, "", "", m.requireSSO, false, m.handleStartProcess)
	server.RouteAPI(m.route("/tasks/:id/context"), "workflow task context", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleTaskContext)
	server.RouteAPI(m.route("/tasks/:id/form-ref"), "workflow task form reference", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleTaskFormRef)
	server.RouteAPI(m.route("/tasks/:id/complete"), "workflow complete task", []string{http.MethodPost}, m.routeRoles, "", "", m.requireSSO, false, m.handleCompleteTask)
	server.RouteAPI(m.route("/org/users/:userId"), "workflow directory user", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleUser)
	server.RouteAPI(m.route("/org/users/:userId/manager"), "workflow directory user manager", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleUserManager)
	server.RouteAPI(m.route("/org/users/:userId/department"), "workflow directory user department", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleUserDepartment)
	server.RouteAPI(m.route("/process-instances/:id/progress-view"), "workflow progress view", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleProgressView)
	server.RouteAPI(m.route("/process-instances/:id/progress-timeline"), "workflow progress timeline", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleProgressTimeline)
	server.RouteAPI(m.route("/biz/:bizId/progress-view"), "workflow progress view by biz id", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleBizProgressView)
	server.RouteAPI(m.route("/biz/:bizId/progress-timeline"), "workflow progress timeline by biz id", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleBizProgressTimeline)
	server.RouteAPI(m.route("/process/instance/:id/definition-xml"), "workflow definition xml", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleDefinitionXML)
	server.RouteAPI(m.route("/process-instances/:id/diagram-view"), "workflow diagram view", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleDiagramView)
	server.RouteAPI(m.route("/process-instances/:id/composite-diagram"), "workflow composite diagram", []string{http.MethodGet}, m.routeRoles, "", "", m.requireSSO, false, m.handleCompositeDiagram)
	server.RouteAPI(m.callbackPath, "workflow flowable callback", []string{http.MethodPost}, m.routeRoles, "", "", false, false, m.handleCallback)
}

func (m *DefaultModule) handleMe(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	response := types.CurrentUserResponse{
		UserID:     user.UserID,
		UserName:   firstNonBlank(user.UserName, user.UserID),
		TenantID:   user.TenantID,
		SystemCode: user.SystemCode,
		Groups:     user.Groups,
		Roles:      user.Roles,
	}
	profile, err := m.directory.GetCurrentUser(c.Request.Context(), user.UserID)
	if err == nil && profile != nil {
		response.DirectoryResolved = true
		response.Directory = profile
		response.UserName = firstNonBlank(profile.DisplayName, response.UserName)
	}
	writeOK(c, response)
}

func (m *DefaultModule) handleTodo(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	page, err := m.flowable.ListTodo(c.Request.Context(), user, parseTaskQuery(c))
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, page)
}

func (m *DefaultModule) handleDone(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	page, err := m.flowable.ListDone(c.Request.Context(), user, parseTaskQuery(c))
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, page)
}

func (m *DefaultModule) handleMyManager(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	manager, err := m.directory.GetManager(c.Request.Context(), user.UserID)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	if manager == nil {
		writeError(c, http.StatusNotFound, "manager not found")
		return
	}
	writeOK(c, manager)
}

func (m *DefaultModule) handleMyDepartment(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	department, err := m.directory.GetDepartment(c.Request.Context(), user.UserID)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	if department == nil {
		writeError(c, http.StatusNotFound, "department not found")
		return
	}
	writeOK(c, department)
}

func (m *DefaultModule) handleUser(c *gin.Context) {
	if _, ok := m.requireUser(c); !ok {
		return
	}
	profile, err := m.directory.GetUser(c.Request.Context(), strings.TrimSpace(c.Param("userId")))
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	if profile == nil {
		writeError(c, http.StatusNotFound, "user not found")
		return
	}
	writeOK(c, profile)
}

func (m *DefaultModule) handleUserManager(c *gin.Context) {
	if _, ok := m.requireUser(c); !ok {
		return
	}
	manager, err := m.directory.GetManager(c.Request.Context(), strings.TrimSpace(c.Param("userId")))
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	if manager == nil {
		writeError(c, http.StatusNotFound, "manager not found")
		return
	}
	writeOK(c, manager)
}

func (m *DefaultModule) handleUserDepartment(c *gin.Context) {
	if _, ok := m.requireUser(c); !ok {
		return
	}
	department, err := m.directory.GetDepartment(c.Request.Context(), strings.TrimSpace(c.Param("userId")))
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	if department == nil {
		writeError(c, http.StatusNotFound, "department not found")
		return
	}
	writeOK(c, department)
}

func (m *DefaultModule) handleTaskContext(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	response, err := m.flowable.GetTaskContext(c.Request.Context(), strings.TrimSpace(c.Param("id")), user)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	if response == nil {
		writeError(c, http.StatusNotFound, "task context not found")
		return
	}
	response.FormRef = m.resolveFormRef(c, response)
	writeOK(c, response)
}

func (m *DefaultModule) handleStartProcess(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	request := &types.StartProcessRequest{}
	if err := c.ShouldBindJSON(request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid start process request")
		return
	}
	if request.Variables == nil {
		request.Variables = map[string]interface{}{}
	}
	if request.Variables["startUserId"] == nil && strings.TrimSpace(user.UserID) != "" {
		request.Variables["startUserId"] = user.UserID
	}
	if request.Variables["tenantId"] == nil && strings.TrimSpace(user.TenantID) != "" {
		request.Variables["tenantId"] = user.TenantID
	}
	if request.Variables["systemCode"] == nil && strings.TrimSpace(user.SystemCode) != "" {
		request.Variables["systemCode"] = user.SystemCode
	}
	response, err := m.flowable.StartProcess(c.Request.Context(), request)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, response)
}

func (m *DefaultModule) handleTaskFormRef(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	response, err := m.flowable.GetTaskContext(c.Request.Context(), strings.TrimSpace(c.Param("id")), user)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	if response == nil {
		writeError(c, http.StatusNotFound, "task context not found")
		return
	}
	formRef := m.resolveFormRef(c, response)
	if formRef == nil {
		formRef = &types.TaskFormReference{
			Configured: false,
			Resolved:   false,
			Fields:     []types.TaskFormFieldReference{},
			Outcomes:   []types.TaskFormOutcomeReference{},
		}
	}
	writeOK(c, formRef)
}

func (m *DefaultModule) handleCompleteTask(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	request := &types.CompleteTaskRequest{}
	if err := c.ShouldBindJSON(request); err != nil && !strings.Contains(strings.ToLower(err.Error()), "eof") {
		writeError(c, http.StatusBadRequest, "invalid complete task request")
		return
	}
	if err := m.flowable.CompleteTask(c.Request.Context(), strings.TrimSpace(c.Param("id")), request, user); err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, gin.H{
		"taskId": strings.TrimSpace(c.Param("id")),
		"status": "completed",
	})
}

func (m *DefaultModule) handleProgressView(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	response, err := m.flowable.GetProgressView(c.Request.Context(), strings.TrimSpace(c.Param("id")), user)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, response)
}

func (m *DefaultModule) handleProgressTimeline(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	response, err := m.flowable.GetProgressTimeline(c.Request.Context(), strings.TrimSpace(c.Param("id")), user)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, response)
}

func (m *DefaultModule) handleBizProgressView(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	response, err := m.flowable.GetProgressViewByBizID(c.Request.Context(), strings.TrimSpace(c.Param("bizId")), user)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, response)
}

func (m *DefaultModule) handleBizProgressTimeline(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	response, err := m.flowable.GetProgressTimelineByBizID(c.Request.Context(), strings.TrimSpace(c.Param("bizId")), user)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, response)
}

func (m *DefaultModule) handleDefinitionXML(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	data, err := m.flowable.GetDefinitionXML(c.Request.Context(), strings.TrimSpace(c.Param("id")), user)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/xml; charset=utf-8", data)
}

func (m *DefaultModule) handleCompositeDiagram(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	response, err := m.flowable.GetDiagramView(c.Request.Context(), strings.TrimSpace(c.Param("id")), user)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, response)
}

func (m *DefaultModule) handleDiagramView(c *gin.Context) {
	user, ok := m.requireUser(c)
	if !ok {
		return
	}
	response, err := m.flowable.GetDiagramView(c.Request.Context(), strings.TrimSpace(c.Param("id")), user)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, response)
}

func (m *DefaultModule) handleCallback(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeError(c, http.StatusBadRequest, "read callback body failed")
		return
	}
	if !m.verifyCallbackSignature(c.Request.Header, body) {
		writeError(c, http.StatusUnauthorized, "invalid callback signature")
		return
	}
	var payload types.FlowableCallbackPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(c, http.StatusBadRequest, "invalid callback payload")
		return
	}
	if err := m.flowable.ProcessCallback(c.Request.Context(), &payload); err != nil {
		writeWorkflowError(c, err)
		return
	}
	writeOK(c, gin.H{"accepted": true})
}

func (m *DefaultModule) requireUser(c *gin.Context) (*workflowcontext.UserContext, bool) {
	user, err := m.resolver.Resolve(c)
	if err != nil {
		writeError(c, http.StatusUnauthorized, err.Error())
		return nil, false
	}
	return user, true
}

func (m *DefaultModule) route(path string) string {
	return joinPath(m.routePrefix, path)
}

func parseTaskQuery(c *gin.Context) *types.TaskQuery {
	return &types.TaskQuery{
		Start:           parseInt(c.Query("start"), 0),
		Size:            parseInt(firstNonBlank(c.Query("size"), c.Query("pageSize")), 20),
		IncludeProgress: parseBool(firstNonBlank(c.Query("includeProgress"), c.Query("withProgress")), false),
		Title:           strings.TrimSpace(firstNonBlank(c.Query("title"), c.Query("keyword"))),
		BizType:         strings.TrimSpace(c.Query("bizType")),
		ProcessDefinitionKey: strings.TrimSpace(firstNonBlank(
			c.Query("processDefinitionKey"),
			c.Query("processKey"),
		)),
		ActivityID: strings.TrimSpace(firstNonBlank(
			c.Query("activityId"),
			c.Query("taskDefinitionKey"),
		)),
		Status:          strings.TrimSpace(c.Query("status")),
		CreatedAfter:    strings.TrimSpace(c.Query("createdAfter")),
		CreatedBefore:   strings.TrimSpace(c.Query("createdBefore")),
		CompletedAfter:  strings.TrimSpace(c.Query("completedAfter")),
		CompletedBefore: strings.TrimSpace(c.Query("completedBefore")),
	}
}

func parseInt(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func parseBool(value string, fallback bool) bool {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func configuredRoles(value string) []string {
	items := splitCSV(value)
	if len(items) == 0 {
		return append([]string(nil), defaultRouteRoles...)
	}
	return items
}

func configuredBool(value string, fallback bool) bool {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func normalizedPrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "/api"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return strings.TrimRight(trimmed, "/")
}

func configuredCallbackPath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "/flowable/callback"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return strings.TrimRight(trimmed, "/")
}

func joinPath(prefix, path string) string {
	left := normalizedPrefix(prefix)
	right := strings.TrimSpace(path)
	if right == "" {
		return left
	}
	if !strings.HasPrefix(right, "/") {
		right = "/" + right
	}
	return left + right
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func writeOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"data":    data,
		"message": "success",
	})
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"data":    nil,
		"message": firstNonBlank(message, http.StatusText(status)),
	})
}

func writeWorkflowError(c *gin.Context, err error) {
	if err == nil {
		writeError(c, http.StatusInternalServerError, "unknown workflow error")
		return
	}
	message := err.Error()
	status := http.StatusInternalServerError
	switch {
	case strings.Contains(strings.ToLower(message), "not found"):
		status = http.StatusNotFound
	case strings.Contains(strings.ToLower(message), "missing"),
		strings.Contains(strings.ToLower(message), "required"),
		strings.Contains(strings.ToLower(message), "invalid"):
		status = http.StatusBadRequest
	}
	writeError(c, status, message)
}

func (m *DefaultModule) verifyCallbackSignature(headers http.Header, body []byte) bool {
	if m == nil || strings.TrimSpace(m.callbackKey) == "" {
		return true
	}
	timestamp := strings.TrimSpace(headers.Get("X-Timestamp"))
	nonce := strings.TrimSpace(headers.Get("X-Nonce"))
	signature := strings.TrimSpace(headers.Get("X-Signature"))
	if timestamp == "" || nonce == "" || signature == "" {
		return false
	}
	content := timestamp + "\n" + nonce + "\n" + string(body)
	mac := hmac.New(sha256.New, []byte(m.callbackKey))
	_, _ = mac.Write([]byte(content))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(strings.ToLower(signature)), []byte(strings.ToLower(expected)))
}

func (m *DefaultModule) resolveFormRef(c *gin.Context, response *types.TaskContextResponse) *types.TaskFormReference {
	if m == nil || m.formref == nil || response == nil {
		return response.FormRef
	}
	formRef, err := m.formref.Resolve(c.Request.Context(), &formref.TaskFormLocator{
		ProcessDefinitionID: response.Task.ProcessDefinitionID,
		ProcessInstanceID:   response.Task.ProcessInstanceID,
		TaskID:              response.Task.TaskID,
		TaskDefinitionKey:   response.Task.ActivityID,
		FormKey:             response.Task.FormKey,
		TenantID:            response.Task.TenantID,
	})
	if err != nil {
		return response.FormRef
	}
	return formRef
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
