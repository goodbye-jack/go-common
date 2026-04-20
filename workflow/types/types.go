package types

type TaskQuery struct {
	Start                int    `json:"start"`
	Size                 int    `json:"size"`
	IncludeProgress      bool   `json:"includeProgress"`
	Title                string `json:"title,omitempty"`
	BizType              string `json:"bizType,omitempty"`
	ProcessDefinitionKey string `json:"processDefinitionKey,omitempty"`
	ActivityID           string `json:"activityId,omitempty"`
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

type ProcessProgressStep struct {
	Order            int      `json:"order"`
	ActivityID       string   `json:"activityId"`
	ActivityName     string   `json:"activityName,omitempty"`
	ActivityType     string   `json:"activityType,omitempty"`
	Status           string   `json:"status"`
	OccurrenceCount  int      `json:"occurrenceCount,omitempty"`
	TaskID           string   `json:"taskId,omitempty"`
	Assignee         string   `json:"assignee,omitempty"`
	Owner            string   `json:"owner,omitempty"`
	CandidateUsers   []string `json:"candidateUsers,omitempty"`
	CandidateGroups  []string `json:"candidateGroups,omitempty"`
	FormKey          string   `json:"formKey,omitempty"`
	StartTime        string   `json:"startTime,omitempty"`
	EndTime          string   `json:"endTime,omitempty"`
	DurationInMillis int64    `json:"durationInMillis,omitempty"`
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
	Items   []ProcessProgressTimelineItem `json:"items,omitempty"`
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
