package admin

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AnnouncementHandler handles admin announcement management
type AnnouncementHandler struct {
	announcementService *service.AnnouncementService
	imageDir            string
}

// NewAnnouncementHandler creates a new admin announcement handler
func NewAnnouncementHandler(announcementService *service.AnnouncementService) *AnnouncementHandler {
	return NewAnnouncementHandlerWithConfig(announcementService, nil)
}

// NewAnnouncementHandlerWithConfig wires the persisted announcement image
// directory from the same data directory already used by other local assets.
func NewAnnouncementHandlerWithConfig(announcementService *service.AnnouncementService, cfg *config.Config) *AnnouncementHandler {
	dataDir := filepath.Join(".", "data")
	if cfg != nil && strings.TrimSpace(cfg.Pricing.DataDir) != "" {
		dataDir = cfg.Pricing.DataDir
	}
	return &AnnouncementHandler{
		announcementService: announcementService,
		imageDir:            filepath.Join(dataDir, "announcement-images"),
	}
}

const (
	announcementImageMaxBytes   = 8 << 20
	announcementImageMaxSide    = 12000
	announcementImageMaxPixels  = 40_000_000
	announcementImagePublicPath = "/api/v1/announcements/assets/"
)

var announcementImageFilenamePattern = regexp.MustCompile(`^[a-f0-9]{32}\.(png|jpg|gif|webp)$`)

type announcementImageUploadResponse struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// UploadImage stores an admin-selected image outside the announcement content
// so persisted Markdown stays small and image bytes survive editor sessions.
// POST /api/v1/admin/announcements/assets
func (h *AnnouncementHandler) UploadImage(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, announcementImageMaxBytes+(1<<20))
	file, _, err := c.Request.FormFile("image")
	if err != nil {
		response.BadRequest(c, "请选择有效的图片文件")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, announcementImageMaxBytes+1))
	if err != nil {
		response.BadRequest(c, "读取图片失败")
		return
	}
	if len(data) > announcementImageMaxBytes {
		response.BadRequest(c, "图片不能超过 8 MB")
		return
	}

	extension, ok := announcementImageType(data)
	if !ok {
		response.BadRequest(c, "仅支持 PNG、JPEG、GIF 或 WebP 图片")
		return
	}
	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || imageConfig.Width <= 0 || imageConfig.Height <= 0 {
		response.BadRequest(c, "图片内容无效")
		return
	}
	if imageConfig.Width > announcementImageMaxSide || imageConfig.Height > announcementImageMaxSide ||
		int64(imageConfig.Width)*int64(imageConfig.Height) > announcementImageMaxPixels {
		response.BadRequest(c, "图片尺寸过大")
		return
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		response.BadRequest(c, "图片内容无效")
		return
	}

	if err := os.MkdirAll(h.imageDir, 0o750); err != nil {
		response.InternalError(c, "创建公告图片目录失败")
		return
	}
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		response.InternalError(c, "生成图片文件名失败")
		return
	}
	filename := hex.EncodeToString(randomBytes) + extension
	if err := writeAnnouncementImage(h.imageDir, filename, data); err != nil {
		response.InternalError(c, "保存公告图片失败")
		return
	}

	response.Created(c, announcementImageUploadResponse{
		URL:    announcementImagePublicPath + filename,
		Width:  imageConfig.Width,
		Height: imageConfig.Height,
	})
}

// ServeImage exposes only uploaded announcement assets. The random filename is
// the unguessable capability: announcement targeting does not protect images,
// so administrators must treat these URLs as public.
// GET /api/v1/announcements/assets/:filename
func (h *AnnouncementHandler) ServeImage(c *gin.Context) {
	filename := c.Param("filename")
	if !announcementImageFilenamePattern.MatchString(filename) {
		c.Status(http.StatusNotFound)
		return
	}
	path := filepath.Join(h.imageDir, filename)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		c.Status(http.StatusNotFound)
		return
	}

	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(path)
}

func announcementImageType(data []byte) (extension string, ok bool) {
	switch http.DetectContentType(data) {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpg", true
	case "image/gif":
		return ".gif", true
	case "image/webp":
		return ".webp", true
	default:
		return "", false
	}
}

func writeAnnouncementImage(dir, filename string, data []byte) error {
	temp, err := os.CreateTemp(dir, ".announcement-image-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()

	if err := temp.Chmod(0o640); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, filepath.Join(dir, filename)); err != nil {
		return fmt.Errorf("rename announcement image: %w", err)
	}
	return nil
}

type CreateAnnouncementRequest struct {
	Title      string                        `json:"title" binding:"required"`
	Content    string                        `json:"content" binding:"required"`
	Status     string                        `json:"status" binding:"omitempty,oneof=draft active archived"`
	NotifyMode string                        `json:"notify_mode" binding:"omitempty,oneof=silent popup"`
	Targeting  service.AnnouncementTargeting `json:"targeting"`
	StartsAt   *int64                        `json:"starts_at"` // Unix seconds, 0/empty = immediate
	EndsAt     *int64                        `json:"ends_at"`   // Unix seconds, 0/empty = never
}

type UpdateAnnouncementRequest struct {
	Title      *string                        `json:"title"`
	Content    *string                        `json:"content"`
	Status     *string                        `json:"status" binding:"omitempty,oneof=draft active archived"`
	NotifyMode *string                        `json:"notify_mode" binding:"omitempty,oneof=silent popup"`
	Targeting  *service.AnnouncementTargeting `json:"targeting"`
	StartsAt   *int64                         `json:"starts_at"` // Unix seconds, 0 = clear
	EndsAt     *int64                         `json:"ends_at"`   // Unix seconds, 0 = clear
}

// List handles listing announcements with filters
// GET /api/v1/admin/announcements
func (h *AnnouncementHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	status := strings.TrimSpace(c.Query("status"))
	search := strings.TrimSpace(c.Query("search"))
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	if len(search) > 200 {
		search = search[:200]
	}

	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}

	items, paginationResult, err := h.announcementService.List(
		c.Request.Context(),
		params,
		service.AnnouncementListFilters{Status: status, Search: search},
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.Announcement, 0, len(items))
	for i := range items {
		out = append(out, *dto.AnnouncementFromService(&items[i]))
	}
	response.Paginated(c, out, paginationResult.Total, page, pageSize)
}

// GetByID handles getting an announcement by ID
// GET /api/v1/admin/announcements/:id
func (h *AnnouncementHandler) GetByID(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	item, err := h.announcementService.GetByID(c.Request.Context(), announcementID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.AnnouncementFromService(item))
}

// Create handles creating a new announcement
// POST /api/v1/admin/announcements
func (h *AnnouncementHandler) Create(c *gin.Context) {
	var req CreateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	input := &service.CreateAnnouncementInput{
		Title:      req.Title,
		Content:    req.Content,
		Status:     req.Status,
		NotifyMode: req.NotifyMode,
		Targeting:  req.Targeting,
		ActorID:    &subject.UserID,
	}

	if req.StartsAt != nil && *req.StartsAt > 0 {
		t := time.Unix(*req.StartsAt, 0)
		input.StartsAt = &t
	}
	if req.EndsAt != nil && *req.EndsAt > 0 {
		t := time.Unix(*req.EndsAt, 0)
		input.EndsAt = &t
	}

	created, err := h.announcementService.Create(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.AnnouncementFromService(created))
}

// Update handles updating an announcement
// PUT /api/v1/admin/announcements/:id
func (h *AnnouncementHandler) Update(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	var req UpdateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	input := &service.UpdateAnnouncementInput{
		Title:      req.Title,
		Content:    req.Content,
		Status:     req.Status,
		NotifyMode: req.NotifyMode,
		Targeting:  req.Targeting,
		ActorID:    &subject.UserID,
	}

	if req.StartsAt != nil {
		if *req.StartsAt == 0 {
			var cleared *time.Time = nil
			input.StartsAt = &cleared
		} else {
			t := time.Unix(*req.StartsAt, 0)
			ptr := &t
			input.StartsAt = &ptr
		}
	}

	if req.EndsAt != nil {
		if *req.EndsAt == 0 {
			var cleared *time.Time = nil
			input.EndsAt = &cleared
		} else {
			t := time.Unix(*req.EndsAt, 0)
			ptr := &t
			input.EndsAt = &ptr
		}
	}

	updated, err := h.announcementService.Update(c.Request.Context(), announcementID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.AnnouncementFromService(updated))
}

// Delete handles deleting an announcement
// DELETE /api/v1/admin/announcements/:id
func (h *AnnouncementHandler) Delete(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	if err := h.announcementService.Delete(c.Request.Context(), announcementID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Announcement deleted successfully"})
}

// ListReadStatus handles listing users read status for an announcement
// GET /api/v1/admin/announcements/:id/read-status
func (h *AnnouncementHandler) ListReadStatus(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "email"),
		SortOrder: c.DefaultQuery("sort_order", "asc"),
	}
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 200 {
		search = search[:200]
	}

	items, paginationResult, err := h.announcementService.ListUserReadStatus(
		c.Request.Context(),
		announcementID,
		params,
		search,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Paginated(c, adminAnnouncementReadResponse(c, items), paginationResult.Total, page, pageSize)
}
