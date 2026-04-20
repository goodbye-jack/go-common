package directorysync

import (
	"fmt"
	"sort"
	"strings"
)

func validateAndOrderDepartments(records []DepartmentRecord) ([]DepartmentRecord, []SyncDetail, error) {
	detailList := make([]SyncDetail, 0)
	ordered := make([]DepartmentRecord, 0, len(records))
	if len(records) == 0 {
		return ordered, detailList, nil
	}
	byCode := make(map[string]DepartmentRecord, len(records))
	incoming := make(map[string]int, len(records))
	children := make(map[string][]string, len(records))
	messages := make([]string, 0)
	for _, record := range records {
		code := strings.TrimSpace(record.Code)
		name := strings.TrimSpace(record.Name)
		if code == "" {
			messages = append(messages, "department code is required")
			continue
		}
		if name == "" {
			messages = append(messages, fmt.Sprintf("department[%s] name is required", code))
			continue
		}
		if _, exists := byCode[code]; exists {
			messages = append(messages, fmt.Sprintf("department[%s] duplicated", code))
			continue
		}
		record.Code = code
		record.Name = name
		record.ParentCode = strings.TrimSpace(record.ParentCode)
		byCode[code] = record
		incoming[code] = 0
	}
	for code, record := range byCode {
		if record.ParentCode == "" {
			continue
		}
		if record.ParentCode == code {
			messages = append(messages, fmt.Sprintf("department[%s] parent can not be itself", code))
			continue
		}
		if _, exists := byCode[record.ParentCode]; !exists {
			messages = append(messages, fmt.Sprintf("department[%s] parent[%s] not found", code, record.ParentCode))
			continue
		}
		children[record.ParentCode] = append(children[record.ParentCode], code)
		incoming[code]++
	}
	if len(messages) > 0 {
		return nil, detailList, ValidationError{Messages: messages}
	}
	queue := make([]string, 0)
	for code, count := range incoming {
		if count == 0 {
			queue = append(queue, code)
		}
	}
	sort.Strings(queue)
	for len(queue) > 0 {
		code := queue[0]
		queue = queue[1:]
		ordered = append(ordered, byCode[code])
		childCodes := append([]string(nil), children[code]...)
		sort.Strings(childCodes)
		for _, childCode := range childCodes {
			incoming[childCode]--
			if incoming[childCode] == 0 {
				queue = append(queue, childCode)
				sort.Strings(queue)
			}
		}
	}
	if len(ordered) != len(byCode) {
		return nil, detailList, ValidationError{Messages: []string{"department parent graph contains cycle"}}
	}
	return ordered, detailList, nil
}

func validatePositions(records []PositionRecord, departments []DepartmentRecord) ([]PositionRecord, []SyncDetail, error) {
	detailList := make([]SyncDetail, 0)
	ordered := make([]PositionRecord, 0, len(records))
	if len(records) == 0 {
		return ordered, detailList, nil
	}
	departmentCodes := make(map[string]struct{}, len(departments))
	for _, department := range departments {
		departmentCodes[department.Code] = struct{}{}
	}
	messages := make([]string, 0)
	positionCodes := make(map[string]struct{}, len(records))
	for _, record := range records {
		code := strings.TrimSpace(record.Code)
		name := strings.TrimSpace(record.Name)
		if code == "" {
			messages = append(messages, "position code is required")
			continue
		}
		if name == "" {
			messages = append(messages, fmt.Sprintf("position[%s] name is required", code))
			continue
		}
		if _, exists := positionCodes[code]; exists {
			messages = append(messages, fmt.Sprintf("position[%s] duplicated", code))
			continue
		}
		record.Code = code
		record.Name = name
		record.DepartmentCode = strings.TrimSpace(record.DepartmentCode)
		if record.DepartmentCode != "" {
			if _, exists := departmentCodes[record.DepartmentCode]; !exists {
				messages = append(messages, fmt.Sprintf("position[%s] department[%s] not found", code, record.DepartmentCode))
				continue
			}
		}
		positionCodes[code] = struct{}{}
		ordered = append(ordered, record)
	}
	sort.SliceStable(ordered, func(index int, otherIndex int) bool {
		return ordered[index].Code < ordered[otherIndex].Code
	})
	if len(messages) > 0 {
		return nil, detailList, ValidationError{Messages: messages}
	}
	return ordered, detailList, nil
}

func validateUsers(records []UserRecord, departments []DepartmentRecord, positions []PositionRecord) ([]UserRecord, []SyncDetail, error) {
	detailList := make([]SyncDetail, 0)
	ordered := make([]UserRecord, 0, len(records))
	if len(records) == 0 {
		return ordered, detailList, nil
	}
	departmentCodes := make(map[string]struct{}, len(departments))
	for _, department := range departments {
		departmentCodes[department.Code] = struct{}{}
	}
	positionCodes := make(map[string]struct{}, len(positions))
	for _, position := range positions {
		positionCodes[position.Code] = struct{}{}
	}
	messages := make([]string, 0)
	userIDs := make(map[string]struct{}, len(records))
	for _, record := range records {
		userID := strings.TrimSpace(record.UserID)
		displayName := strings.TrimSpace(record.DisplayName)
		if userID == "" {
			messages = append(messages, "user id is required")
			continue
		}
		if displayName == "" {
			messages = append(messages, fmt.Sprintf("user[%s] display name is required", userID))
			continue
		}
		if _, exists := userIDs[userID]; exists {
			messages = append(messages, fmt.Sprintf("user[%s] duplicated", userID))
			continue
		}
		record.UserID = userID
		record.UserName = firstNonBlank(strings.TrimSpace(record.UserName), userID)
		record.DisplayName = displayName
		record.DepartmentCodes = normalizeStringSlice(record.DepartmentCodes)
		record.PositionCodes = normalizeStringSlice(record.PositionCodes)
		for _, departmentCode := range record.DepartmentCodes {
			if _, exists := departmentCodes[departmentCode]; !exists {
				messages = append(messages, fmt.Sprintf("user[%s] department[%s] not found", userID, departmentCode))
			}
		}
		for _, positionCode := range record.PositionCodes {
			if _, exists := positionCodes[positionCode]; !exists {
				messages = append(messages, fmt.Sprintf("user[%s] position[%s] not found", userID, positionCode))
			}
		}
		userIDs[userID] = struct{}{}
		ordered = append(ordered, record)
	}
	sort.SliceStable(ordered, func(index int, otherIndex int) bool {
		return ordered[index].UserID < ordered[otherIndex].UserID
	})
	if len(messages) > 0 {
		return nil, detailList, ValidationError{Messages: messages}
	}
	for _, record := range ordered {
		if len(record.DepartmentCodes) == 0 {
			detailList = append(detailList, SyncDetail{
				RecordType: RecordTypeUser,
				RecordKey:  record.UserID,
				RecordName: record.DisplayName,
				Action:     "validate",
				Status:     DetailStatusWarning,
				Message:    "user has no department binding",
			})
		}
	}
	return ordered, detailList, nil
}

func validateGroups(records []GroupRecord, users []UserRecord) ([]GroupRecord, []SyncDetail, error) {
	detailList := make([]SyncDetail, 0)
	ordered := make([]GroupRecord, 0, len(records))
	if len(records) == 0 {
		return ordered, detailList, nil
	}
	userIDs := make(map[string]struct{}, len(users))
	for _, user := range users {
		userIDs[user.UserID] = struct{}{}
	}
	messages := make([]string, 0)
	groupCodes := make(map[string]struct{}, len(records))
	for _, record := range records {
		code := strings.TrimSpace(record.Code)
		name := strings.TrimSpace(record.Name)
		if code == "" {
			messages = append(messages, "group code is required")
			continue
		}
		if name == "" {
			messages = append(messages, fmt.Sprintf("group[%s] name is required", code))
			continue
		}
		if _, exists := groupCodes[code]; exists {
			messages = append(messages, fmt.Sprintf("group[%s] duplicated", code))
			continue
		}
		record.Code = code
		record.Name = name
		record.MemberUserIDs = normalizeStringSlice(record.MemberUserIDs)
		for _, userID := range record.MemberUserIDs {
			if _, exists := userIDs[userID]; !exists {
				messages = append(messages, fmt.Sprintf("group[%s] user[%s] not found", code, userID))
			}
		}
		groupCodes[code] = struct{}{}
		ordered = append(ordered, record)
	}
	sort.SliceStable(ordered, func(index int, otherIndex int) bool {
		return ordered[index].Code < ordered[otherIndex].Code
	})
	if len(messages) > 0 {
		return nil, detailList, ValidationError{Messages: messages}
	}
	for _, record := range ordered {
		if len(record.MemberUserIDs) == 0 {
			detailList = append(detailList, SyncDetail{
				RecordType: RecordTypeGroup,
				RecordKey:  record.Code,
				RecordName: record.Name,
				Action:     "validate",
				Status:     DetailStatusWarning,
				Message:    "group has no active members and will be removed from directory if it exists",
			})
		}
	}
	return ordered, detailList, nil
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
