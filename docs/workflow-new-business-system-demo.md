# 新业务系统接入工作流详细演示

## 1. 文档目的

这份文档不是再讲原则，而是做一遍“一个全新的业务系统如何从 0 接入 `go-common/workflow`”的完整演示。

你可以把它理解成：

- 一份给架构师看的落地样例
- 一份给后端组照着改的接入样例
- 一份给前端组照着搭的页面样例
- 一份给测试组照着验的联调样例

## 2. 演示场景设定

假设现在有一个新的业务系统：

- 系统名称：`museum-collection-service`
- 业务名称：馆藏文物借展审批
- 业务类型：`collection_loan`
- 流程定义 key：`collection_loan_flow`

业务规则很简单：

1. 申请人发起借展申请
2. 馆内初审
3. 部门审批
4. 馆领导确认
5. 流程结束

这套系统要接入 `go-common/workflow`，并满足下面这些目标：

- 业务数据存自己的业务表
- 前端页面自己渲染
- 待办、已办、流程图、时间线走工作流通用能力
- 节点表单只作为字段参考

## 3. 最终架构图

可以把最终结构理解成下面这样：

### 3.1 业务系统自己的部分

- 业务表：`biz_collection_loan`
- 业务创建接口
- 我的申请接口
- 业务详情接口
- 任务处理页
- 我的申请页

### 3.2 `go-common/workflow` 提供的通用部分

- 工作流上下文解析
- 发起流程
- 待办
- 已办
- 任务上下文
- 表单引用解析
- 流程图
- 时间线
- LDAP 组织信息

### 3.3 Flowable 提供的底层能力

- 流程定义
- 流程实例
- 任务实例
- 历史任务
- 流程变量
- 表单模型

## 4. 第一步：设计业务表

新业务系统首先要有自己的业务表。

推荐表结构：

```sql
CREATE TABLE biz_collection_loan (
  biz_id varchar(64) NOT NULL PRIMARY KEY COMMENT '业务单号',
  biz_type varchar(64) NOT NULL COMMENT '业务类型',
  title varchar(255) NOT NULL COMMENT '业务标题',
  applicant varchar(128) NOT NULL COMMENT '申请人姓名',
  applicant_user_id varchar(64) NOT NULL COMMENT '申请人账号',
  relic_name varchar(255) NOT NULL COMMENT '文物名称',
  loan_reason text COMMENT '借展原因',
  loan_start_date datetime COMMENT '借展开始时间',
  loan_end_date datetime COMMENT '借展结束时间',
  status varchar(64) NOT NULL COMMENT '业务状态',
  process_instance_id varchar(64) DEFAULT NULL COMMENT '流程实例ID',
  started_by_user_id varchar(64) NOT NULL COMMENT '发起人账号',
  started_by_user_name varchar(128) DEFAULT NULL COMMENT '发起人姓名',
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL,
  KEY idx_collection_loan_process_instance_id (process_instance_id),
  KEY idx_collection_loan_starter_updated (started_by_user_id, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 4.1 为什么必须保留这张业务表

因为下面这些能力都必须来自业务系统自己：

- 我的申请
- 业务详情
- 业务字段回显
- 业务扩展状态
- 附件和其他领域字段

这部分不能交给 Flowable 底表。

## 5. 第二步：准备业务状态枚举

业务系统要自己维护业务状态，不要直接把 Flowable 状态原样透出成业务状态。

推荐定义：

```go
const (
	StatusDraft      = "DRAFT"
	StatusSubmitted  = "SUBMITTED"
	StatusRunning    = "RUNNING"
	StatusApproved   = "APPROVED"
	StatusRejected   = "REJECTED"
	StatusCompleted  = "COMPLETED"
	StatusStartFail  = "START_FAILED"
)
```

建议规则：

- 业务状态是业务系统自己的概念
- 流程状态是工作流的概念
- 页面可以同时展示这两种状态，但后端不要混为一谈

## 6. 第三步：接入 `go-common/workflow`

业务服务启动时，把工作流模块注册进去。

示例：

```go
package main

import (
	commonhttp "github.com/goodbye-jack/go-common/http"
	workflowapi "github.com/goodbye-jack/go-common/workflow/api"
)

func main() {
	server := commonhttp.NewHTTPServer("museum-collection-service")

	module, err := workflowapi.NewDefaultModuleFromConfig()
	if err != nil {
		panic(err)
	}
	module.Register(server)

	registerBizRoutes(server)
	server.Prepare()
	server.Run(":9090")
}
```

### 6.1 这一步完成后自动获得的能力

注册成功后，业务服务自动具备：

- `/api/me/tasks/todo`
- `/api/me/tasks/done`
- `/api/tasks/{id}/context`
- `/api/tasks/{id}/complete`
- `/api/process-instances/{id}/progress-view`
- `/api/process-instances/{id}/progress-timeline`
- `/api/biz/{bizId}/progress-view`
- `/api/biz/{bizId}/progress-timeline`
- `/api/process/instance/{id}/definition-xml`

这部分不用业务组再自己重写。

## 7. 第四步：实现业务创建接口

新系统需要一个业务创建接口，例如：

- `POST /api/collection-loans`

### 7.1 请求体示例

```json
{
  "title": "省博借展审批-20260311-001",
  "applicant": "王敏",
  "relicName": "青铜鼎",
  "loanReason": "参加省级联合展览",
  "loanStartDate": "2026-04-01 00:00:00",
  "loanEndDate": "2026-04-30 23:59:59"
}
```

### 7.2 后端处理顺序

后端必须严格按下面顺序处理：

1. 校验请求参数
2. 生成 `bizId`
3. 写入业务表
4. 构造 `StartProcessRequest`
5. 调工作流发起接口
6. 回写 `processInstanceId`
7. 返回给前端

### 7.3 示例代码结构

```go
func (s *Server) createCollectionLoan(w http.ResponseWriter, r *http.Request) {
	currentUser := s.mustCurrentUser(r)

	bizID := generateBizID()
	title := req.Title

	record := &CollectionLoan{
		BizID:             bizID,
		BizType:           "collection_loan",
		Title:             title,
		Applicant:         req.Applicant,
		ApplicantUserID:   currentUser.UserID,
		RelicName:         req.RelicName,
		LoanReason:        req.LoanReason,
		LoanStartDate:     req.LoanStartDate,
		LoanEndDate:       req.LoanEndDate,
		Status:            StatusSubmitted,
		StartedByUserID:   currentUser.UserID,
		StartedByUserName: currentUser.UserName,
	}

	// 1. 先写业务表
	if err := db.Create(record).Error; err != nil {
		// 返回错误
	}

	// 2. 再发起流程
	resp, err := workflow.StartProcess(ctx, &workflowtypes.StartProcessRequest{
		ProcessDefinitionKey: "collection_loan_flow",
		BizID:                bizID,
		BizType:              "collection_loan",
		Title:                title,
		Name:                 title,
		Variables: map[string]interface{}{
			"payloadRef": bizID,
			"tenantId":   currentUser.TenantID,
			"systemCode": currentUser.SystemCode,
		},
	})

	if err != nil {
		db.Model(record).Updates(map[string]interface{}{
			"status": StatusStartFail,
		})
		// 返回错误
	}

	// 3. 成功后回写流程实例ID
	db.Model(record).Updates(map[string]interface{}{
		"status":              StatusRunning,
		"process_instance_id": resp.ProcessInstanceID,
	})
}
```

### 7.4 这一步最容易犯的错

- 没有先写业务表
- 没有传 `bizId`
- 没有传 `title`
- 没有把 `processInstanceId` 回写业务表
- 发起失败时没有标记业务状态

## 8. 第五步：实现“我的申请”

这个接口一定是业务接口，不是工作流接口。

推荐接口：

- `GET /api/collection-loans/my`

### 8.1 查询逻辑

1. 按当前登录人的 `started_by_user_id` 查业务表
2. 对每条记录按 `bizId` 查 `progress-view`
3. 把进度摘要挂到业务记录上返回

### 8.2 返回结构建议

```json
{
  "items": [
    {
      "bizId": "LOAN-20260311-001",
      "title": "省博借展审批-20260311-001",
      "status": "RUNNING",
      "processInstanceId": "xxx",
      "progress": {
        "status": "RUNNING",
        "currentActivityNames": ["馆内初审"],
        "currentAssignees": [],
        "currentCandidateUsers": ["test"],
        "currentCandidateGroups": []
      }
    }
  ]
}
```

### 8.3 页面展示要求

页面至少要展示：

- 标题
- 业务状态
- 流程状态
- 当前节点
- 当前办理人或候选范围

## 9. 第六步：直接使用待办和已办接口

业务系统不需要自己重新发明一套待办和已办逻辑。

直接使用：

- `GET /api/me/tasks/todo`
- `GET /api/me/tasks/done`

### 9.1 待办列表推荐展示字段

- 任务名称
- 业务单号
- 业务标题
- 当前节点
- 当前办理人
- 候选人
- 候选组
- 处理按钮

### 9.2 已办列表推荐展示字段

- 任务名称
- 业务单号
- 业务标题
- 完成时间
- 查看流程按钮

## 10. 第七步：任务详情页接入

任务详情页核心接口：

- `GET /api/tasks/{taskId}/context`

### 10.1 页面需要展示的模块

1. 任务基础信息
2. 业务详情
3. 流程状态摘要
4. 内嵌流程图
5. 流程时间线
6. 节点表单参考
7. 业务处理区

### 10.2 关键字段说明

任务上下文返回里前端最少要使用：

- `task.taskId`
- `task.activityId`
- `task.activityName`
- `task.processInstanceId`
- `task.bizId`
- `task.title`
- `formRef`
- `variables`

## 11. 第八步：内嵌流程图和时间线

前端要使用两类接口：

### 11.1 流程图

- `GET /api/process/instance/{id}/definition-xml`

要求：

- 页面内嵌展示
- 图内不能出现滚动条
- 整图完整显示

### 11.2 时间线

- `GET /api/process-instances/{id}/progress-timeline`
- 或 `GET /api/biz/{bizId}/progress-timeline`

要求：

- 要能看到已完成节点
- 要能看到处理中节点
- 要能看到正向未来节点
- 办理人和候选范围必须显示准确

## 12. 第九步：处理任务

任务处理接口：

- `POST /api/tasks/{taskId}/complete`

### 12.1 处理页推荐字段

假设“馆内初审”节点需要这些字段：

- `result`
- `comment`
- `needLeaderReview`

### 12.2 推荐请求结构

```json
{
  "activityId": "task_internal_review",
  "result": "APPROVED",
  "comment": "符合借展要求",
  "payloadRef": "LOAN-20260311-001",
  "variables": {
    "internalReviewResult": "APPROVED",
    "needLeaderReview": true
  }
}
```

### 12.3 后端映射规则

后端要做两件事：

1. 把页面字段映射为流程变量
2. 更新业务表状态

例如：

- `APPROVED` -> `RUNNING`
- `REJECTED` -> `REJECTED`

## 13. 第十步：人员显示规则

前端必须统一遵守这条规则，否则业务用户会误解流程状态：

1. 有 `assignee` 时，显示真实办理人
2. 无 `assignee` 但有 `candidateUsers` 或 `candidateGroups` 时，显示“待认领”
3. 只有系统节点才显示“系统”

推荐展示文案：

- `办理人：张三`
- `待认领 / 候选人：张三 / 候选组：馆内审批组`
- `系统`

## 14. 第十一步：标题不丢失的实现要求

标题要做到双保存：

### 14.1 存在业务表

业务表中必须有：

- `title`

### 14.2 存在流程变量

发起流程时必须传：

- `title`

### 14.3 查询时双兜底

待办、已办、任务上下文查询时，标题读取顺序建议是：

1. 先读工作流返回的 `title`
2. 如果为空，再按 `bizId` 或 `payloadRef` 回查业务表标题

## 15. 第十二步：联调顺序

建议联调严格按下面顺序推进：

1. 发起新流程
2. 看“我的申请”
3. 看“我的待办”
4. 打开任务详情
5. 提交任务处理
6. 看“我的已办”
7. 看流程图
8. 看时间线
9. 重启后再验一次

## 16. 第十三步：联调中最常见问题

最常见的问题通常是下面这些：

- 发起流程成功，但没有回写 `processInstanceId`
- 我的申请正常，但待办里没有标题
- 当前节点正确，但当前人员显示成“系统”
- 流程图能打开，但图内出现滚动条
- 时间线里缺少正在处理的节点
- 服务重启后业务列表为空

## 17. 第十四步：最终验收标准

只有当下面这些条件全部成立时，才认为这个新业务系统已经成功接入：

- 发起成功
- 我的申请正常
- 我的待办正常
- 我的已办正常
- 标题不丢
- 当前节点正确
- 当前人员正确
- 流程图正常
- 时间线正常
- 任务处理正常
- 重启后数据正常

## 18. 建议如何使用这份演示文档

建议你后面把这份文档直接发给新的业务组，并要求他们：

1. 先按这份文档做技术方案对齐
2. 再按这份文档拆开发任务
3. 最后按这份文档走联调和验收

这样每个新业务系统接入时，基本都能走同一条标准路径，不用再从头解释一次。
