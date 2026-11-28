package controller

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const ticketImageMaxSize = 15 << 20 // 15 MB
var ticketImageAllowedTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// ---------------------------------------------------------------------------
// User side
// ---------------------------------------------------------------------------

type createTicketRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// GetUserTickets 当前用户的工单列表
func GetUserTickets(c *gin.Context) {
	userId := c.GetInt("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tickets, total, err := model.GetUserTickets(userId, page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": tickets,
			"total": total,
			"page":  page,
			"size":  pageSize,
		},
	})
}

// CreateTicket 用户发起工单
func CreateTicket(c *gin.Context) {
	userId := c.GetInt("id")
	var req createTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorT(c, "Invalid request")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)

	ticket, err := model.CreateTicket(userId, req.Title, req.Content)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccessT(c, "Ticket created", ticket)
}

// GetTicketDetail 工单详情（含消息列表）。用户只能看自己的，管理员能看所有。
// 打开详情即视为已读——更新该角色的 last_read_at。
func GetTicketDetail(c *gin.Context) {
	userId := c.GetInt("id")
	role := c.GetInt("role")
	id, _ := strconv.Atoi(c.Param("id"))

	ticket, err := model.GetTicketById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if role < common.RoleAdminUser && ticket.UserId != userId {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Permission denied"})
		return
	}

	messages, err := model.GetTicketMessages(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 自己的工单按"用户"身份记已读（管理员看自己提的工单也一样）；
	// 管理员经用户路由查看他人工单时才记为 admin 已读。
	readRole := model.TicketSenderRoleUser
	if ticket.UserId != userId {
		readRole = model.TicketSenderRoleAdmin
	}
	_ = model.MarkTicketRead(id, readRole)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"ticket":   ticket,
			"messages": messages,
		},
	})
}

// ReplyTicket 用户回复自己的工单；如果工单已关闭，回复会把它重新打开。
func ReplyTicket(c *gin.Context) {
	userId := c.GetInt("id")
	id, _ := strconv.Atoi(c.Param("id"))

	ticket, err := model.GetTicketById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if ticket.UserId != userId {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Permission denied"})
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorT(c, "Invalid request")
		return
	}
	req.Content = strings.TrimSpace(req.Content)

	msg, err := model.AddTicketMessage(id, userId, model.TicketSenderRoleUser, req.Content, ticket.Status == model.TicketStatusClosed)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccessT(c, "Reply sent", msg)
}

// CloseTicket 用户关闭自己的工单
func CloseTicket(c *gin.Context) {
	userId := c.GetInt("id")
	id, _ := strconv.Atoi(c.Param("id"))

	if err := model.CloseTicket(id, userId, false); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccessT(c, "Ticket closed", nil)
}

// ---------------------------------------------------------------------------
// Admin side
// ---------------------------------------------------------------------------

// AdminGetTickets 管理员工单列表
func AdminGetTickets(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.DefaultQuery("status", "")
	keyword := strings.TrimSpace(c.DefaultQuery("keyword", ""))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tickets, total, err := model.GetAllTickets(status, keyword, page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": tickets,
			"total": total,
			"page":  page,
			"size":  pageSize,
		},
	})
}

// AdminGetTicketDetail 管理员查看工单详情；打开即标记 admin 已读。
func AdminGetTicketDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	ticket, err := model.GetTicketById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	messages, err := model.GetTicketMessages(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.MarkTicketRead(id, model.TicketSenderRoleAdmin)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"ticket":   ticket,
			"messages": messages,
		},
	})
}

// AdminReplyTicket 管理员回复工单；如果工单已关闭，回复会把它重新打开。
func AdminReplyTicket(c *gin.Context) {
	adminId := c.GetInt("id")
	id, _ := strconv.Atoi(c.Param("id"))

	ticket, err := model.GetTicketById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorT(c, "Invalid request")
		return
	}
	req.Content = strings.TrimSpace(req.Content)

	msg, err := model.AddTicketMessage(id, adminId, model.TicketSenderRoleAdmin, req.Content, ticket.Status == model.TicketStatusClosed)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccessT(c, "Reply sent", msg)
}

// GetUnreadCount 返回未读工单数（给侧边栏红点用）：
// user_count 为自己提交的工单中有新回复的数量；admin_count 仅管理员返回，为待处理的用户消息数量。
func GetUnreadCount(c *gin.Context) {
	userId := c.GetInt("id")
	isAdmin := c.GetInt("role") >= common.RoleAdminUser
	userCount, err := model.CountUserUnreadTickets(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var adminCount int64
	if isAdmin {
		adminCount, err = model.CountAdminUnreadTickets()
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user_count":  userCount,
			"admin_count": adminCount,
		},
	})
}

// AdminCloseTicket 管理员关闭工单
func AdminCloseTicket(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := model.CloseTicket(id, 0, true); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccessT(c, "Ticket closed", nil)
}

// ---------------------------------------------------------------------------
// Image upload (shared)
// ---------------------------------------------------------------------------

// UploadTicketImage 上传工单内嵌图片，返回可访问的 URL。
// 文件存到 ./upload/tickets/YYYYMM/<hash>.<ext>，通过 /api/ticket-image/:month/:name 访问。
func UploadTicketImage(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, ticketImageMaxSize)

	file, header, err := c.Request.FormFile("image")
	if err != nil {
		common.ApiErrorT(c, "No image file provided")
		return
	}
	defer file.Close()

	if header.Size > ticketImageMaxSize {
		common.ApiErrorT(c, "Image exceeds 15 MB limit")
		return
	}

	ext, ok := ticketImageAllowedTypes[header.Header.Get("Content-Type")]
	if !ok {
		common.ApiErrorT(c, "Only JPEG, PNG, GIF and WebP images are allowed")
		return
	}

	// Read a small prefix to sniff the real type; the declared Content-Type is
	// not trustworthy on its own.
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	detected := http.DetectContentType(buf[:n])
	if _, allowed := ticketImageAllowedTypes[detected]; !allowed {
		common.ApiErrorT(c, "Only JPEG, PNG, GIF and WebP images are allowed")
		return
	}
	if _, err := file.Seek(0, 0); err != nil {
		common.ApiError(c, err)
		return
	}

	dir := filepath.Join("upload", "tickets", time.Now().Format("200601"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		common.ApiError(c, err)
		return
	}

	name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	dst := filepath.Join(dir, name)
	out, err := os.Create(dst)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccessT(c, "Uploaded", gin.H{
		"url": fmt.Sprintf("/api/ticket-image/%s/%s", time.Now().Format("200601"), name),
	})
}

// ServeTicketImage 公开访问工单图片（登录用户）。不签名，路径遍历被 Gin 的
// 路径参数自动限制在单段文件名上。
func ServeTicketImage(c *gin.Context) {
	month := c.Param("month")
	name := c.Param("name")
	// Strict allow-list so nothing outside upload/tickets can be served.
	if !isSafeTicketImageName(month) || !isSafeTicketImageName(name) {
		c.Status(http.StatusBadRequest)
		return
	}
	path := filepath.Join("upload", "tickets", month, name)
	if _, err := os.Stat(path); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(path)
}

func isSafeTicketImageName(s string) bool {
	if s == "" || strings.Contains(s, "/") || strings.Contains(s, "\\") || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
