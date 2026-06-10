package types

type TaskQuery struct {
	Start                int    `json:"start"`
	Size                 int    `json:"size"`
	IncludeProgress      bool   `json:"includeProgress"`
	Title                string `json:"title,omitempty"`
	BizType              string `json:"bizType,omitempty"`
	ProcessDefinitionKey string `json:"processDefinitionKey,omitempty"`
	ActivityID           string `json:"activityId,omitempty"`
	ActionType           string `json:"actionType,omitempty"`
	Status               string `json:"status,omitempty"`
	CreatedAfter         string `json:"createdAfter,omitempty"`
	CreatedBefore        string `json:"createdBefore,omitempty"`
	CompletedAfter       string `json:"completedAfter,omitempty"`
	CompletedBefore      string `json:"completedBefore,omitempty"`
}

type StartProcessRequest struct {
	ProcessDefinitionID  string                 `json:"processDefinitionId,omitempty"`
	ProcessDefinitionKey string                 `json:"processDefinitionKey,omitempty"`
	BusinessKey          string                 `json:"businessKey,omitempty"`
	BizID                string                 `json:"bizId,omitempty"`
	BizType              string                 `json:"bizType,omitempty"`
	Title                string                 `json:"title,omitempty"`
	Name                 string                 `json:"name,omitempty"`
	Variables            map[string]interface{} `json:"variables,omitempty"`
}

type StartProcessResponse struct {
	ProcessInstanceID   string `json:"processInstanceId"`
	ProcessDefinitionID string `json:"processDefinitionId,omitempty"`
	BusinessKey         string `json:"businessKey,omitempty"`
	TenantID            string `json:"tenantId,omitempty"`
}

type FlowableCallbackPayload struct {
	EventID             string                 `json:"eventId"`
	EventType           string                 `json:"eventType"`
	EventTime           string                 `json:"eventTime,omitempty"`
	ProcessInstanceID   string                 `json:"processInstanceId"`
	ProcessDefinitionID string                 `json:"processDefinitionId,omitempty"`
	ActivityID          string                 `json:"activityId,omitempty"`
	TaskID              string                 `json:"taskId,omitempty"`
	BizID               string                 `json:"bizId,omitempty"`
	Variables           map[string]interface{} `json:"variables,omitempty"`
}

type CompleteTaskRequest struct {
	ActivityID    string                 `json:"activityId,omitempty"`
	Result        string                 `json:"result,omitempty"`
	Comment       string                 `json:"comment,omitempty"`
	ReworkComment string                 `json:"reworkComment,omitempty"`
	PayloadRef    string                 `json:"payloadRef,omitempty"`
	NeedExpert    bool                   `json:"needExpert,omitempty"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
}

// TaskDelegateRequest 表示“委派任务”请求。
// Assignee 是要委派给的目标办理人工作流用户 ID。
type TaskDelegateRequest struct {
	Assignee string `json:"assignee,omitempty"`
}

// TaskResolveRequest 表示“解决委派”请求。
// Variables 是受托人处理时回写到流程实例的变量集合。
type TaskResolveRequest struct {
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// TaskTransferRequest 表示“转办任务”请求。
// Assignee 是转办后的新办理人工作流用户 ID。
// Reason 是转办原因，用于接口审计和注释说明。
type TaskTransferRequest struct {
	Assignee string `json:"assignee,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// TaskActionResponse 表示任务管理动作执行结果。
// Status 取值由具体动作决定，例如 claimed、unclaimed、delegated、resolved、transferred。
type TaskActionResponse struct {
	TaskID          string `json:"taskId"`
	Status          string `json:"status"`
	Assignee        string `json:"assignee,omitempty"`
	Owner           string `json:"owner,omitempty"`
	DelegationState string `json:"delegationState,omitempty"`
}

type AssignmentUserContext struct {
	UserID     string   `json:"userId,omitempty"`
	UserName   string   `json:"userName,omitempty"`
	TenantID   string   `json:"tenantId,omitempty"`
	SystemCode string   `json:"systemCode,omitempty"`
	Groups     []string `json:"groups,omitempty"`
	Roles      []string `json:"roles,omitempty"`
}

type AssignmentResolveRequest struct {
	Action               string                 `json:"action,omitempty"`
	ProcessDefinitionID  string                 `json:"processDefinitionId,omitempty"`
	ProcessDefinitionKey string                 `json:"processDefinitionKey,omitempty"`
	ProcessInstanceID    string                 `json:"processInstanceId,omitempty"`
	BusinessKey          string                 `json:"businessKey,omitempty"`
	BizID                string                 `json:"bizId,omitempty"`
	BizType              string                 `json:"bizType,omitempty"`
	Title                string                 `json:"title,omitempty"`
	Name                 string                 `json:"name,omitempty"`
	TaskID               string                 `json:"taskId,omitempty"`
	ActivityID           string                 `json:"activityId,omitempty"`
	Result               string                 `json:"result,omitempty"`
	Comment              string                 `json:"comment,omitempty"`
	ReworkComment        string                 `json:"reworkComment,omitempty"`
	PayloadRef           string                 `json:"payloadRef,omitempty"`
	NeedExpert           bool                   `json:"needExpert,omitempty"`
	User                 AssignmentUserContext  `json:"user"`
	Task                 *TaskInfo              `json:"task,omitempty"`
	Business             *TaskBusinessContext   `json:"business,omitempty"`
	Variables            map[string]interface{} `json:"variables,omitempty"`
	CurrentVariables     map[string]interface{} `json:"currentVariables,omitempty"`
}

type AssignmentResolveResponse struct {
	Variables       map[string]interface{} `json:"variables,omitempty"`
	Assignee        string                 `json:"assignee,omitempty"`
	CandidateUsers  []string               `json:"candidateUsers,omitempty"`
	CandidateGroups []string               `json:"candidateGroups,omitempty"`
}

type TaskInfo struct {
	TaskID                string                  `json:"taskId"`
	TaskName              string                  `json:"taskName,omitempty"`
	ActivityID            string                  `json:"activityId"`
	ActivityName          string                  `json:"activityName,omitempty"`
	ProcessInstanceID     string                  `json:"processInstanceId,omitempty"`
	RootProcessInstanceID string                  `json:"rootProcessInstanceId,omitempty"`
	ProcessDefinitionID   string                  `json:"processDefinitionId,omitempty"`
	BizID                 string                  `json:"bizId,omitempty"`
	BizType               string                  `json:"bizType,omitempty"`
	Title                 string                  `json:"title,omitempty"`
	PayloadRef            string                  `json:"payloadRef,omitempty"`
	CreatedAt             string                  `json:"createdAt,omitempty"`
	CompletedAt           string                  `json:"completedAt,omitempty"`
	Assignee              string                  `json:"assignee,omitempty"`
	Owner                 string                  `json:"owner,omitempty"`
	DelegationState       string                  `json:"delegationState,omitempty"`
	TenantID              string                  `json:"tenantId,omitempty"`
	FormKey               string                  `json:"formKey,omitempty"`
	FormName              string                  `json:"formName,omitempty"`
	UIPage                string                  `json:"uiPage,omitempty"`
	Progress              *ProcessProgressSummary `json:"progress,omitempty"`
}

type TaskPage struct {
	Items []TaskInfo `json:"items"`
	Total int64      `json:"total"`
	Start int        `json:"start"`
	Size  int        `json:"size"`
}

type TaskBusinessContext struct {
	BizID      string `json:"bizId,omitempty"`
	BizType    string `json:"bizType,omitempty"`
	Title      string `json:"title,omitempty"`
	PayloadRef string `json:"payloadRef,omitempty"`
	SystemCode string `json:"systemCode,omitempty"`
	TenantID   string `json:"tenantId,omitempty"`
}

type TaskFormFieldReference struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required"`
}

type TaskFormOutcomeReference struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type TaskFormReference struct {
	Configured   bool                       `json:"configured"`
	Resolved     bool                       `json:"resolved"`
	Source       string                     `json:"source,omitempty"`
	FormKey      string                     `json:"formKey,omitempty"`
	FormName     string                     `json:"formName,omitempty"`
	DeploymentID string                     `json:"deploymentId,omitempty"`
	ResourceName string                     `json:"resourceName,omitempty"`
	Fields       []TaskFormFieldReference   `json:"fields,omitempty"`
	Outcomes     []TaskFormOutcomeReference `json:"outcomes,omitempty"`
}

type TaskContextResponse struct {
	Task      TaskInfo               `json:"task"`
	Business  TaskBusinessContext    `json:"business,omitempty"`
	Variables map[string]interface{} `json:"variables,omitempty"`
	FormRef   *TaskFormReference     `json:"formRef,omitempty"`
}

type DirectoryDepartment struct {
	DepartmentID         string `json:"departmentId"`
	DepartmentName       string `json:"departmentName"`
	DN                   string `json:"dn,omitempty"`
	ParentDepartmentID   string `json:"parentDepartmentId,omitempty"`
	ParentDepartmentName string `json:"parentDepartmentName,omitempty"`
}

type DirectoryPosition struct {
	PositionID   string `json:"positionId"`
	PositionName string `json:"positionName"`
	DN           string `json:"dn,omitempty"`
}

type DirectoryUserSummary struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email,omitempty"`
	DN          string `json:"dn,omitempty"`
}

type DirectoryUserProfile struct {
	UserID      string                `json:"userId"`
	Username    string                `json:"username"`
	DisplayName string                `json:"displayName"`
	GivenName   string                `json:"givenName,omitempty"`
	FamilyName  string                `json:"familyName,omitempty"`
	Email       string                `json:"email,omitempty"`
	Mobile      string                `json:"mobile,omitempty"`
	DN          string                `json:"dn,omitempty"`
	Title       string                `json:"title,omitempty"`
	Department  *DirectoryDepartment  `json:"department,omitempty"`
	Position    *DirectoryPosition    `json:"position,omitempty"`
	Manager     *DirectoryUserSummary `json:"manager,omitempty"`
}

type CurrentUserResponse struct {
	UserID            string                `json:"userId"`
	UserName          string                `json:"userName"`
	TenantID          string                `json:"tenantId"`
	SystemCode        string                `json:"systemCode"`
	Groups            []string              `json:"groups,omitempty"`
	Roles             []string              `json:"roles,omitempty"`
	DirectoryResolved bool                  `json:"directoryResolved"`
	Directory         *DirectoryUserProfile `json:"directory,omitempty"`
}

type ProcessProgressSummary struct {
	ProcessInstanceID      string   `json:"processInstanceId"`
	ProcessDefinitionID    string   `json:"processDefinitionId,omitempty"`
	ProcessDefinitionKey   string   `json:"processDefinitionKey,omitempty"`
	BizID                  string   `json:"bizId,omitempty"`
	BizType                string   `json:"bizType,omitempty"`
	Title                  string   `json:"title,omitempty"`
	TenantID               string   `json:"tenantId,omitempty"`
	SystemCode             string   `json:"systemCode,omitempty"`
	StartUserID            string   `json:"startUserId,omitempty"`
	StartUserName          string   `json:"startUserName,omitempty"`
	Status                 string   `json:"status"`
	ProgressPercent        int      `json:"progressPercent"`
	CompletedTaskCount     int      `json:"completedTaskCount"`
	TotalTaskCount         int      `json:"totalTaskCount"`
	CurrentTaskCount       int      `json:"currentTaskCount"`
	StartTime              string   `json:"startTime,omitempty"`
	EndTime                string   `json:"endTime,omitempty"`
	CurrentActivityIDs     []string `json:"currentActivityIds,omitempty"`
	CurrentActivityNames   []string `json:"currentActivityNames,omitempty"`
	CurrentAssignees       []string `json:"currentAssignees,omitempty"`
	CurrentCandidateUsers  []string `json:"currentCandidateUsers,omitempty"`
	CurrentCandidateGroups []string `json:"currentCandidateGroups,omitempty"`
	DiagramURL             string   `json:"diagramUrl,omitempty"`
}

type WorkflowTaskRecordItem struct {
	Sequence              int    `json:"sequence"`
	RecordID              uint   `json:"recordId,omitempty"`
	ActionType            string `json:"actionType"`
	Time                  string `json:"time,omitempty"`
	ProcessInstanceID     string `json:"processInstanceId,omitempty"`
	RootProcessInstanceID string `json:"rootProcessInstanceId,omitempty"`
	ProcessDefinitionID   string `json:"processDefinitionId,omitempty"`
	ProcessDefinitionKey  string `json:"processDefinitionKey,omitempty"`
	TaskID                string `json:"taskId,omitempty"`
	TaskName              string `json:"taskName,omitempty"`
	ActivityID            string `json:"activityId,omitempty"`
	ActivityName          string `json:"activityName,omitempty"`
	BizID                 string `json:"bizId,omitempty"`
	BizType               string `json:"bizType,omitempty"`
	Title                 string `json:"title,omitempty"`
	TenantID              string `json:"tenantId,omitempty"`
	SystemCode            string `json:"systemCode,omitempty"`
	OperatorUserID        string `json:"operatorUserId,omitempty"`
	OperatorUserName      string `json:"operatorUserName,omitempty"`
	FromAssignee          string `json:"fromAssignee,omitempty"`
	ToAssignee            string `json:"toAssignee,omitempty"`
	FromOwner             string `json:"fromOwner,omitempty"`
	ToOwner               string `json:"toOwner,omitempty"`
	Comment               string `json:"comment,omitempty"`
	Reason                string `json:"reason,omitempty"`
	Source                string `json:"source,omitempty"`
}

type WorkflowTaskRecordPage struct {
	Items []WorkflowTaskRecordItem `json:"items"`
	Total int64                    `json:"total"`
	Start int                      `json:"start"`
	Size  int                      `json:"size"`
}

type ProcessProgressExecution struct {
	Sequence          int                      `json:"sequence"`
	TaskID            string                   `json:"taskId,omitempty"`
	ProcessInstanceID string                   `json:"processInstanceId,omitempty"`
	Status            string                   `json:"status"`
	Assignee          string                   `json:"assignee,omitempty"`
	Owner             string                   `json:"owner,omitempty"`
	CandidateUsers    []string                 `json:"candidateUsers"`
	CandidateGroups   []string                 `json:"candidateGroups"`
	StartTime         string                   `json:"startTime,omitempty"`
	EndTime           string                   `json:"endTime,omitempty"`
	DurationInMillis  int64                    `json:"durationInMillis,omitempty"`
	Records           []WorkflowTaskRecordItem `json:"records"`
}

type ProcessProgressStep struct {
	Order            int                        `json:"order"`
	ActivityID       string                     `json:"activityId"`
	ActivityName     string                     `json:"activityName,omitempty"`
	ActivityType     string                     `json:"activityType,omitempty"`
	Status           string                     `json:"status"`
	OccurrenceCount  int                        `json:"occurrenceCount,omitempty"`
	TaskID           string                     `json:"taskId,omitempty"`
	Assignee         string                     `json:"assignee,omitempty"`
	Owner            string                     `json:"owner,omitempty"`
	CandidateUsers   []string                   `json:"candidateUsers,omitempty"`
	CandidateGroups  []string                   `json:"candidateGroups,omitempty"`
	FormKey          string                     `json:"formKey,omitempty"`
	StartTime        string                     `json:"startTime,omitempty"`
	EndTime          string                     `json:"endTime,omitempty"`
	DurationInMillis int64                      `json:"durationInMillis,omitempty"`
	Executions       []ProcessProgressExecution `json:"executions"`
}

type ProcessProgressViewResponse struct {
	Summary              ProcessProgressSummary `json:"summary"`
	Steps                []ProcessProgressStep  `json:"steps,omitempty"`
	CompletedActivityIDs []string               `json:"completedActivityIds,omitempty"`
	CurrentActivityIDs   []string               `json:"currentActivityIds,omitempty"`
}

type ProcessProgressTimelineItem struct {
	Sequence          int      `json:"sequence"`
	ItemType          string   `json:"itemType"`
	Status            string   `json:"status"`
	Occurrence        int      `json:"occurrence,omitempty"`
	ActivityID        string   `json:"activityId,omitempty"`
	ActivityName      string   `json:"activityName,omitempty"`
	ActivityType      string   `json:"activityType,omitempty"`
	TaskID            string   `json:"taskId,omitempty"`
	TaskDefinitionKey string   `json:"taskDefinitionKey,omitempty"`
	Assignee          string   `json:"assignee,omitempty"`
	Owner             string   `json:"owner,omitempty"`
	CandidateUsers    []string `json:"candidateUsers,omitempty"`
	CandidateGroups   []string `json:"candidateGroups,omitempty"`
	FormKey           string   `json:"formKey,omitempty"`
	StartTime         string   `json:"startTime,omitempty"`
	EndTime           string   `json:"endTime,omitempty"`
	DurationInMillis  int64    `json:"durationInMillis,omitempty"`
}

type ProcessProgressTimelineResponse struct {
	Summary ProcessProgressSummary        `json:"summary"`
	Items   []ProcessProgressTimelineItem `json:"items"`
}

const (
	TaskActionTypeStartProcess              = "START_PROCESS"
	ProcessActionTimelineItemTypeTaskAction = "TASK_ACTION"
	TaskActionTypeClaim                     = "CLAIM"
	TaskActionTypeUnclaim                   = "UNCLAIM"
	TaskActionTypeDelegate                  = "DELEGATE"
	TaskActionTypeResolve                   = "RESOLVE"
	TaskActionTypeTransfer                  = "TRANSFER"
	TaskActionTypeComplete                  = "COMPLETE"
)

// ProcessActionTimelineSummary 复用流程摘要语义。
// 动作时间线本质上仍然挂载在同一个流程实例范围上，因此摘要字段与进度时间线保持一致。
type ProcessActionTimelineSummary = ProcessProgressSummary

// ProcessActionTimelineItem 表示一次任务管理动作记录。
// 它只描述“谁来办”的动作轨迹，不负责表达 BPMN 节点推进路径。
type ProcessActionTimelineItem struct {
	Sequence              int    `json:"sequence"`
	ItemType              string `json:"itemType"`
	ActionType            string `json:"actionType"`
	Time                  string `json:"time,omitempty"`
	ProcessInstanceID     string `json:"processInstanceId,omitempty"`
	RootProcessInstanceID string `json:"rootProcessInstanceId,omitempty"`
	TaskID                string `json:"taskId,omitempty"`
	TaskName              string `json:"taskName,omitempty"`
	ActivityID            string `json:"activityId,omitempty"`
	ActivityName          string `json:"activityName,omitempty"`
	OperatorUserID        string `json:"operatorUserId,omitempty"`
	OperatorUserName      string `json:"operatorUserName,omitempty"`
	FromAssignee          string `json:"fromAssignee,omitempty"`
	ToAssignee            string `json:"toAssignee,omitempty"`
	FromOwner             string `json:"fromOwner,omitempty"`
	ToOwner               string `json:"toOwner,omitempty"`
	Reason                string `json:"reason,omitempty"`
	CommentID             string `json:"commentId,omitempty"`
}

// ProcessActionTimelineResponse 表示流程动作时间线。
// 与 progress-timeline 并列存在，语义上只承载任务动作审计，不混入节点推进事件。
type ProcessActionTimelineResponse struct {
	Summary ProcessActionTimelineSummary `json:"summary"`
	Items   []ProcessActionTimelineItem  `json:"items"`
}

type ProcessTaskRecordResponse struct {
	Summary ProcessProgressSummary   `json:"summary"`
	Items   []WorkflowTaskRecordItem `json:"items"`
}

type ProcessCompositeDiagramChild struct {
	CallActivityID       string `json:"callActivityId"`
	CallActivityName     string `json:"callActivityName"`
	ProcessInstanceID    string `json:"processInstanceId,omitempty"`
	ProcessDefinitionID  string `json:"processDefinitionId,omitempty"`
	ProcessDefinitionKey string `json:"processDefinitionKey,omitempty"`
	XML                  string `json:"xml"`
}

type ProcessCompositeDiagramResponse struct {
	ProcessInstanceID string                         `json:"processInstanceId"`
	Mode              string                         `json:"mode"`
	Composite         bool                           `json:"composite"`
	ParentXML         string                         `json:"parentXml"`
	Children          []ProcessCompositeDiagramChild `json:"children,omitempty"`
}
