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
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/goodbye-jack/go-common/workflow/assignment"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/directory"
	"github.com/goodbye-jack/go-common/workflow/engine/flowable"
	"github.com/goodbye-jack/go-common/workflow/formref"
	"github.com/goodbye-jack/go-common/workflow/identity"
	"github.com/goodbye-jack/go-common/workflow/types"
)

const (
	configAPIEnabled    = "workflow.api.enabled"
	configAPIPrefix     = "workflow.api.prefix"
	configAPILogRoutes  = "workflow.api.log_routes"
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
	assignment   assignment.Service
	flowable     flowable.Client
	formref      formref.Service
	routePrefix  string
	routeRoles   []string
	logRoutes    bool
	requireSSO   bool
	callbackPath string
	callbackKey  string
}

type workflowRouteDefinition struct {
	Path       string
	Tips       string
	Methods    []string
	RequireSSO bool
	Handler    gin.HandlerFunc
}

func EnabledFromConfig() bool {
	return config.GetConfigBool(configAPIEnabled)
}

func RegisterFromConfig(server *commonhttp.HTTPServer) error {
	if !EnabledFromConfig() {
		log.Infof("【workflow】未启用标准接口注册，配置项 %s=false", configAPIEnabled)
		return nil
	}
	module, provider, assignmentProvider, err := newModuleWithProviderFromConfig()
	if err != nil {
		return err
	}
	logStartupSummary(module, provider, assignmentProvider)
	module.Register(server)
	return nil
}

func MustRegisterFromConfig(server *commonhttp.HTTPServer) {
	if err := RegisterFromConfig(server); err != nil {
		log.Fatalf("workflow standard module register failed: %v", err)
	}
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
		assignment:   assignment.NewNoopService(),
		flowable:     flowableClient,
		routePrefix:  normalizedPrefix(config.GetConfigString(configAPIPrefix)),
		routeRoles:   configuredRoles(config.GetConfigString(configAPIRoles)),
		logRoutes:    configuredBool(config.GetConfigString(configAPILogRoutes), true),
		requireSSO:   configuredBool(config.GetConfigString(configAPISSOEnabled), true),
		callbackPath: configuredCallbackPath(config.GetConfigString(configCallbackPath)),
		callbackKey:  strings.TrimSpace(config.GetConfigString(configCallbackKey)),
	}, nil
}

func NewDefaultModuleFromConfig() (*DefaultModule, error) {
	resolver := workflowcontext.NewDefaultResolver()
	directoryService, _, err := directory.NewServiceFromConfig()
	if err != nil {
		return nil, err
	}
	assignmentService, _, err := assignment.NewServiceFromConfig()
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
	module.assignment = assignmentService
	module.formref = formref.NewFlowableService(flowableClient)
	return module, nil
}

func newModuleWithProviderFromConfig() (*DefaultModule, string, string, error) {
	resolver := workflowcontext.NewDefaultResolver()
	directoryService, provider, err := directory.NewServiceFromConfig()
	if err != nil {
		return nil, provider, "", err
	}
	assignmentService, assignmentProvider, err := assignment.NewServiceFromConfig()
	if err != nil {
		return nil, provider, assignmentProvider, err
	}
	flowableClient, err := flowable.NewRESTClientFromConfig()
	if err != nil {
		return nil, provider, assignmentProvider, err
	}
	module, err := NewDefaultModule(resolver, directoryService, flowableClient)
	if err != nil {
		return nil, provider, assignmentProvider, err
	}
	module.assignment = assignmentService
	module.formref = formref.NewFlowableService(flowableClient)
	return module, provider, assignmentProvider, nil
}

func (m *DefaultModule) WithFormRefService(service formref.Service) *DefaultModule {
	if m == nil {
		return nil
	}
	m.formref = service
	return m
}

func (m *DefaultModule) WithAssignmentService(service assignment.Service) *DefaultModule {
	if m == nil {
		return nil
	}
	if service != nil {
		m.assignment = service
	}
	return m
}

func (m *DefaultModule) Register(server *commonhttp.HTTPServer) {
	if server == nil {
		return
	}
	routes := m.routeDefinitions()
	for _, route := range routes {
		server.RouteAPI(route.Path, route.Tips, route.Methods, m.routeRoles, "", "", route.RequireSSO, false, route.Handler)
	}
	m.logRegisteredRoutes(routes)
}

func (m *DefaultModule) routeDefinitions() []workflowRouteDefinition {
	return []workflowRouteDefinition{
		{Path: m.route("/me"), Tips: "workflow current user", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleMe},
		{Path: m.route("/me/tasks/todo"), Tips: "workflow todo tasks", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleTodo},
		{Path: m.route("/me/tasks/done"), Tips: "workflow done tasks", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleDone},
		{Path: m.route("/me/manager"), Tips: "workflow current user manager", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleMyManager},
		{Path: m.route("/me/department"), Tips: "workflow current user department", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleMyDepartment},
		{Path: m.route("/process/start"), Tips: "workflow start process", Methods: []string{http.MethodPost}, RequireSSO: m.requireSSO, Handler: m.handleStartProcess},
		{Path: m.route("/tasks/:id/context"), Tips: "workflow task context", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleTaskContext},
		{Path: m.route("/tasks/:id/form-ref"), Tips: "workflow task form reference", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleTaskFormRef},
		{Path: m.route("/tasks/:id/complete"), Tips: "workflow complete task", Methods: []string{http.MethodPost}, RequireSSO: m.requireSSO, Handler: m.handleCompleteTask},
		{Path: m.route("/org/users/:userId"), Tips: "workflow directory user", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleUser},
		{Path: m.route("/org/users/:userId/manager"), Tips: "workflow directory user manager", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleUserManager},
		{Path: m.route("/org/users/:userId/department"), Tips: "workflow directory user department", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleUserDepartment},
		{Path: m.route("/process-instances/:id/progress-view"), Tips: "workflow progress view", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleProgressView},
		{Path: m.route("/process-instances/:id/progress-timeline"), Tips: "workflow progress timeline", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleProgressTimeline},
		{Path: m.route("/biz/:bizId/progress-view"), Tips: "workflow progress view by biz id", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleBizProgressView},
		{Path: m.route("/biz/:bizId/progress-timeline"), Tips: "workflow progress timeline by biz id", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleBizProgressTimeline},
		{Path: m.route("/process/instance/:id/definition-xml"), Tips: "workflow definition xml", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleDefinitionXML},
		{Path: m.route("/process-instances/:id/diagram-view"), Tips: "workflow diagram view", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleDiagramView},
		{Path: m.route("/process-instances/:id/composite-diagram"), Tips: "workflow composite diagram", Methods: []string{http.MethodGet}, RequireSSO: m.requireSSO, Handler: m.handleCompositeDiagram},
		{Path: m.callbackPath, Tips: "workflow flowable callback", Methods: []string{http.MethodPost}, RequireSSO: false, Handler: m.handleCallback},
	}
}

func (m *DefaultModule) logRegisteredRoutes(routes []workflowRouteDefinition) {
	if m == nil || !m.logRoutes || len(routes) == 0 {
		return
	}
	total := 0
	for _, route := range routes {
		total += len(route.Methods)
	}
	log.Info("======================================================================")
	log.Infof("=== WORKFLOW ROUTES REGISTERED | total=%d | prefix=%s | callback=%s ===", total, m.routePrefix, m.callbackPath)
	log.Info("======================================================================")
	index := 0
	for _, route := range routes {
		for _, method := range route.Methods {
			index++
			ssoFlag := "N"
			if route.RequireSSO {
				ssoFlag = "Y"
			}
			log.Infof("=== WORKFLOW ROUTE [%02d/%02d] %-6s %-48s | SSO=%s | %s", index, total, strings.ToUpper(method), route.Path, ssoFlag, route.Tips)
		}
	}
	log.Info("======================================================================")
	log.Info("=== WORKFLOW ROUTE LOG END ===========================================")
	log.Info("======================================================================")
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
	if request.Variables["starterId"] == nil && strings.TrimSpace(user.UserID) != "" {
		request.Variables["starterId"] = user.UserID
	}
	if request.Variables["starterName"] == nil && strings.TrimSpace(user.UserName) != "" {
		request.Variables["starterName"] = user.UserName
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
	if m.assignment != nil {
		resolved, err := m.assignment.ResolveStart(c.Request.Context(), buildStartAssignmentRequest(user, request))
		if err != nil {
			writeWorkflowError(c, err)
			return
		}
		request.Variables = assignment.MergeResolvedVariables(request.Variables, resolved)
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
	if request.Variables == nil {
		request.Variables = map[string]interface{}{}
	}
	if m.assignment != nil {
		taskContext, err := m.flowable.GetTaskContext(c.Request.Context(), strings.TrimSpace(c.Param("id")), user)
		if err != nil {
			writeWorkflowError(c, err)
			return
		}
		resolved, err := m.assignment.ResolveComplete(
			c.Request.Context(),
			buildCompleteAssignmentRequest(user, strings.TrimSpace(c.Param("id")), request, taskContext),
		)
		if err != nil {
			writeWorkflowError(c, err)
			return
		}
		request.Variables = assignment.MergeResolvedVariables(request.Variables, resolved)
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
	case strings.Contains(strings.ToLower(message), "not configured"),
		strings.Contains(strings.ToLower(message), "unavailable"):
		status = http.StatusServiceUnavailable
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

func buildStartAssignmentRequest(user *workflowcontext.UserContext, request *types.StartProcessRequest) *types.AssignmentResolveRequest {
	if request == nil {
		request = &types.StartProcessRequest{}
	}
	return &types.AssignmentResolveRequest{
		Action:               "start",
		ProcessDefinitionID:  strings.TrimSpace(request.ProcessDefinitionID),
		ProcessDefinitionKey: strings.TrimSpace(request.ProcessDefinitionKey),
		BusinessKey:          strings.TrimSpace(firstNonBlank(request.BusinessKey, request.BizID)),
		BizID:                strings.TrimSpace(request.BizID),
		BizType:              strings.TrimSpace(request.BizType),
		Title:                strings.TrimSpace(request.Title),
		Name:                 strings.TrimSpace(request.Name),
		User:                 toAssignmentUser(user),
		Variables:            cloneVariables(request.Variables),
	}
}

func buildCompleteAssignmentRequest(user *workflowcontext.UserContext, taskID string, request *types.CompleteTaskRequest, taskContext *types.TaskContextResponse) *types.AssignmentResolveRequest {
	if request == nil {
		request = &types.CompleteTaskRequest{}
	}
	resolvedTaskID := strings.TrimSpace(taskID)
	currentVariables := map[string]interface{}{}
	var task *types.TaskInfo
	var business *types.TaskBusinessContext
	processDefinitionID := ""
	processInstanceID := ""
	businessKey := ""
	bizID := ""
	bizType := ""
	title := ""
	if taskContext != nil {
		copyTask := taskContext.Task
		task = &copyTask
		if taskContext.Business.BizID != "" || taskContext.Business.BizType != "" || taskContext.Business.Title != "" || taskContext.Business.PayloadRef != "" || taskContext.Business.SystemCode != "" || taskContext.Business.TenantID != "" {
			copyBusiness := taskContext.Business
			business = &copyBusiness
		}
		currentVariables = cloneVariables(taskContext.Variables)
		if resolvedTaskID == "" {
			resolvedTaskID = strings.TrimSpace(taskContext.Task.TaskID)
		}
		processDefinitionID = strings.TrimSpace(taskContext.Task.ProcessDefinitionID)
		processInstanceID = strings.TrimSpace(taskContext.Task.ProcessInstanceID)
		businessKey = strings.TrimSpace(firstNonBlank(taskContext.Business.BizID, taskContext.Task.BizID, taskContext.Task.PayloadRef))
		bizID = strings.TrimSpace(firstNonBlank(taskContext.Business.BizID, taskContext.Task.BizID))
		bizType = strings.TrimSpace(firstNonBlank(taskContext.Business.BizType, taskContext.Task.BizType))
		title = strings.TrimSpace(firstNonBlank(taskContext.Business.Title, taskContext.Task.Title))
	}
	return &types.AssignmentResolveRequest{
		Action:              "complete",
		ProcessDefinitionID: processDefinitionID,
		ProcessInstanceID:   processInstanceID,
		BusinessKey:         businessKey,
		BizID:               bizID,
		BizType:             bizType,
		Title:               title,
		TaskID:              resolvedTaskID,
		ActivityID:          strings.TrimSpace(firstNonBlank(request.ActivityID, activityIDFromTask(task))),
		Result:              strings.TrimSpace(request.Result),
		Comment:             strings.TrimSpace(request.Comment),
		ReworkComment:       strings.TrimSpace(request.ReworkComment),
		PayloadRef:          strings.TrimSpace(request.PayloadRef),
		NeedExpert:          resolveNeedExpertForAssignment(request, currentVariables),
		User:                toAssignmentUser(user),
		Task:                task,
		Business:            business,
		Variables:           cloneVariables(request.Variables),
		CurrentVariables:    currentVariables,
	}
}

func resolveNeedExpertForAssignment(request *types.CompleteTaskRequest, currentVariables map[string]interface{}) bool {
	if request == nil {
		return boolValueFromMap(currentVariables, "needExpert")
	}
	if value, ok := boolValueFromAny(request.Variables, "needExpert"); ok {
		return value
	}
	if request.NeedExpert {
		return true
	}
	return boolValueFromMap(currentVariables, "needExpert")
}

func boolValueFromMap(values map[string]interface{}, key string) bool {
	value, ok := boolValueFromAny(values, key)
	return ok && value
}

func boolValueFromAny(values map[string]interface{}, key string) (bool, bool) {
	if len(values) == 0 {
		return false, false
	}
	raw, exists := values[key]
	if !exists {
		return false, false
	}
	switch current := raw.(type) {
	case bool:
		return current, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(current))
		if err != nil {
			return false, false
		}
		return parsed, true
	default:
		return false, false
	}
}

func toAssignmentUser(user *workflowcontext.UserContext) types.AssignmentUserContext {
	if user == nil {
		return types.AssignmentUserContext{}
	}
	return types.AssignmentUserContext{
		UserID:     strings.TrimSpace(user.UserID),
		UserName:   strings.TrimSpace(user.UserName),
		TenantID:   strings.TrimSpace(user.TenantID),
		SystemCode: strings.TrimSpace(user.SystemCode),
		Groups:     append([]string(nil), user.Groups...),
		Roles:      append([]string(nil), user.Roles...),
	}
}

func cloneVariables(source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return map[string]interface{}{}
	}
	target := make(map[string]interface{}, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}

func activityIDFromTask(task *types.TaskInfo) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.ActivityID)
}

func logStartupSummary(module *DefaultModule, provider, assignmentProvider string) {
	if module == nil {
		return
	}
	userIDStrategy := firstNonBlank(config.GetConfigString("workflow.context.user_id_strategy"), "raw")
	formRefDBInstance := strings.TrimSpace(config.GetConfigString("workflow.formref.db_instance"))
	assignmentAssigneeKey := firstNonBlank(config.GetConfigString("workflow.assignment.variable_keys.assignee"), "nextAssignee")
	assignmentCandidateUsersKey := firstNonBlank(config.GetConfigString("workflow.assignment.variable_keys.candidate_users"), "nextCandidateUsers")
	assignmentCandidateGroupsKey := firstNonBlank(config.GetConfigString("workflow.assignment.variable_keys.candidate_groups"), "nextCandidateGroups")
	normalizer := identity.NewNormalizerFromConfig()
	log.Infof("【workflow】标准接口启用 | prefix=%s | sso=%t | user_id_strategy=%s | directory_provider=%s | assignment_provider=%s | flowable_base_url=%s | assignment_base_url=%s | variable_keys=%s/%s/%s | role_alias_count=%d | group_alias_count=%d | formref_db_instance=%s",
		module.routePrefix,
		module.requireSSO,
		userIDStrategy,
		firstNonBlank(provider, "none"),
		firstNonBlank(assignmentProvider, "none"),
		strings.TrimSpace(config.GetConfigString("workflow.flowable.base_url")),
		firstNonBlank(strings.TrimSpace(config.GetConfigString("workflow.assignment.http.base_url")), "(empty)"),
		assignmentAssigneeKey,
		assignmentCandidateUsersKey,
		assignmentCandidateGroupsKey,
		normalizer.RoleAliasCount(),
		normalizer.GroupAliasCount(),
		firstNonBlank(formRefDBInstance, "(empty)"),
	)
	if firstNonBlank(provider, "none") == "none" {
		log.Warn("【workflow】directory provider=none，组织目录接口可访问但会返回未配置错误")
	}
	if isNoneProvider(assignmentProvider) {
		log.Warn("【workflow】assignment provider=none，如 BPMN 依赖 nextAssignee/nextCandidateUsers/nextCandidateGroups，请改为显式配置 assignment provider")
	}
	if userIDStrategy == "raw" {
		log.Warn("【workflow】user_id_strategy=raw，请确认业务登录态中的 UserID 与 Flowable assignee/candidateUser 完全一致")
	}
	if formRefDBInstance == "" {
		log.Warn("【workflow】workflow.formref.db_instance 未配置，任务表单引用解析能力可能受限")
	}
}

func isNoneProvider(provider string) bool {
	provider = strings.TrimSpace(provider)
	if provider == "" || provider == "none" {
		return true
	}
	return strings.HasSuffix(provider, ":none")
}
