package handler

import "github.com/meridian-labs/meridian/internal/service"

// overlayErrorDetails 渲染契约定义的 overlay_invalid 错误详情列表：
// 一基的 {line, column, message} 条目加上 issue code。
func overlayErrorDetails(err *service.OverlayInvalidError) map[string]any {
	type overlayIssueDetail struct {
		Line    int    `json:"line"`
		Column  int    `json:"column"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	issues := make([]overlayIssueDetail, 0, len(err.Errors))
	for _, issue := range err.Errors {
		issues = append(issues, overlayIssueDetail{
			Line: issue.Line, Column: issue.Column, Message: issue.Message, Code: issue.Code,
		})
	}
	return map[string]any{"errors": issues}
}
