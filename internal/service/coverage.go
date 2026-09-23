package service

import (
	"context"
	"encoding/json/v2"
	"time"
	"unicode/utf8"
	"uuid"
)

// 本文件承载 M6 收口的轻量目录协作用例：服务收藏、用户偏好、视图覆盖、
// 标签字典与服务评论。各操作此前为 501 桩，M6 落地真实实现。

const (
	// maxTagNameRunes 是标签名称的最大字符数。
	maxTagNameRunes = 64
	// maxTagColorRunes 是标签颜色的最大字符数。
	maxTagColorRunes = 16
	// tagColorDefault 是标签创建时未显式提供颜色的缺省值，与 DB 列 DEFAULT 同源。
	tagColorDefault = "#64748b"
	// maxServiceCommentBodyRunes 是服务评论正文的最大字符数。
	maxServiceCommentBodyRunes = 2000
	// viewOverrideOrdDefault 是新建视图覆盖的默认排序号。
	viewOverrideOrdDefault = 0
)

// TagRecord 是一个标签字典条目投影。
type TagRecord struct {
	// ID 是标签的标识。
	ID uuid.UUID
	// Name 是租户内唯一的标签名称。
	Name string
	// Color 是标签展示颜色。
	Color string
	// Description 是标签描述（可为空）。
	Description *string
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// NewTag 承载一次标签插入。
type NewTag struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是标签的标识。
	ID uuid.UUID
	// Name 是标签名称。
	Name string
	// Color 是标签颜色（可为空）。
	Color *string
	// Description 是标签描述（可为空）。
	Description *string
}

// TagPatch 承载一次标签 PATCH 的显式字段。
type TagPatch struct {
	// Name 是替换名称（可为空）。
	Name *string
	// Color 是替换颜色（可为空）。
	Color *string
	// Description 是替换描述（可为空）。
	Description *string
	// SetDescription 表示是否显式提供 Description（区分省略与置空）。
	SetDescription bool
}

// CommentRecord 是一个服务评论投影。
type CommentRecord struct {
	// ID 是评论的标识。
	ID uuid.UUID
	// ServiceID 是被评论的服务标识。
	ServiceID uuid.UUID
	// AuthorID 是评论作者标识。
	AuthorID uuid.UUID
	// Body 是评论正文。
	Body string
	// AuthorDisplayName 是作者展示名。
	AuthorDisplayName string
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// UserPreferencesRecord 是当前用户的偏好投影。
type UserPreferencesRecord struct {
	// Locale 是界面语言。
	Locale string
	// Theme 是颜色模式。
	Theme string
	// DefaultViews 是从上下文映射到默认视图标识的 JSON。
	DefaultViews []byte
	// Revision 是乐观并发版本号。
	Revision int64
}

// UserPreferencesPatch 承载一次偏好 PATCH 的显式字段。
type UserPreferencesPatch struct {
	// Locale 是替换语言（可为空）。
	Locale *string
	// Theme 是替换主题（可为空）。
	Theme *string
	// DefaultViews 是替换默认视图 JSON（可为空）。
	DefaultViews []byte
}

// ViewOverrideRecord 是一个租户级视图覆盖投影。
type ViewOverrideRecord struct {
	// ViewID 是被覆盖的内置视图标识。
	ViewID string
	// Enabled 表示该视图是否启用。
	Enabled bool
	// Ord 是视图在租户内的展示排序。
	Ord int
	// DefaultOptions 是租户级默认参数覆盖 JSON。
	DefaultOptions []byte
	// Revision 是乐观并发版本号。
	Revision int64
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// TenantExportRecord 是一个租户导出任务记录。
type TenantExportRecord struct {
	// ID 是导出任务的标识。
	ID uuid.UUID
	// Status 是导出状态。
	Status string
	// RequestedBy 是发起导出的用户标识。
	RequestedBy uuid.UUID
	// CreatedAt 是创建时间。
	CreatedAt time.Time
}

// CoverageStore 是轻量目录协作的持久化边界。
type CoverageStore interface {
	// GetServiceBySlug 按 slug 返回租户内一条活跃服务。
	GetServiceBySlug(context.Context, uuid.UUID, string) (ServiceRecord, error)
	// UpsertServiceStar 幂等收藏一个服务。
	UpsertServiceStar(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	// DeleteServiceStar 取消收藏一个服务。
	DeleteServiceStar(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	// GetServiceStar 返回服务是否被某用户收藏。
	GetServiceStar(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
	// GetUserPreferences 返回某用户偏好。
	GetUserPreferences(context.Context, uuid.UUID) (UserPreferencesRecord, error)
	// UpdateUserPreferences 在 If-Match 下更新用户偏好。
	UpdateUserPreferences(context.Context, uuid.UUID, int64, UserPreferencesPatch) (UserPreferencesRecord, error)
	// ListViewOverrides 返回租户内全部视图覆盖。
	ListViewOverrides(context.Context, uuid.UUID) ([]ViewOverrideRecord, error)
	// GetViewOverride 返回一条视图覆盖。
	GetViewOverride(context.Context, uuid.UUID, string) (ViewOverrideRecord, error)
	// UpsertViewOverride 幂等设置一条视图覆盖。
	UpsertViewOverride(context.Context, uuid.UUID, string, bool, int, []byte) (ViewOverrideRecord, error)
	// DeleteViewOverride 删除一条视图覆盖。
	DeleteViewOverride(context.Context, uuid.UUID, string) error
	// ListTags 返回租户内全部标签。
	ListTags(context.Context, uuid.UUID) ([]TagRecord, error)
	// GetTag 返回一条标签。
	GetTag(context.Context, uuid.UUID, uuid.UUID) (TagRecord, error)
	// CreateTag 创建一条标签。
	CreateTag(context.Context, NewTag) (TagRecord, error)
	// UpdateTag 在 If-Match 下更新一条标签。
	UpdateTag(context.Context, uuid.UUID, uuid.UUID, int64, TagPatch) (TagRecord, error)
	// DeleteTag 在 If-Match 下删除一条标签。
	DeleteTag(context.Context, uuid.UUID, uuid.UUID, int64) error
	// ListServiceComments 返回一个服务下的评论分页。
	ListServiceComments(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]CommentRecord, int64, error)
	// CreateServiceComment 创建一条服务评论。
	CreateServiceComment(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, string) (CommentRecord, error)
	// CreateTenantExport 创建一条租户导出任务记录。
	CreateTenantExport(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (TenantExportRecord, error)
}

// Coverage 协调轻量目录协作用例。
type Coverage struct {
	store      CoverageStore
	identities IdentityStore
}

// NewCoverage 构造 M6 目录协作用例。
func NewCoverage(store CoverageStore, identities IdentityStore) *Coverage {
	return &Coverage{store: store, identities: identities}
}

// StarService 收藏一个服务。
func (coverage *Coverage) StarService(ctx context.Context, actor Principal, tenantSlug, serviceSlug string) (bool, error) {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return false, err
	}
	service, err := coverage.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return false, err
	}
	if err := coverage.store.UpsertServiceStar(ctx, membership.TenantID, service.ID, membership.UserID); err != nil {
		return false, err
	}
	return true, nil
}

// UnstarService 取消收藏一个服务。
func (coverage *Coverage) UnstarService(ctx context.Context, actor Principal, tenantSlug, serviceSlug string) (bool, error) {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return false, err
	}
	service, err := coverage.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return false, err
	}
	if err := coverage.store.DeleteServiceStar(ctx, membership.TenantID, service.ID, membership.UserID); err != nil {
		return false, err
	}
	return false, nil
}

// GetMyPreferences 返回当前用户的偏好。
func (coverage *Coverage) GetMyPreferences(ctx context.Context, actor Principal) (UserPreferencesRecord, error) {
	return coverage.store.GetUserPreferences(ctx, actor.User.ID)
}

// UpdateMyPreferences 在 If-Match 下更新当前用户的偏好。
func (coverage *Coverage) UpdateMyPreferences(ctx context.Context, actor Principal, etag string, patch UserPreferencesPatch) (UserPreferencesRecord, error) {
	current, err := coverage.store.GetUserPreferences(ctx, actor.User.ID)
	if err != nil {
		return UserPreferencesRecord{}, err
	}
	expectedRevision, err := parseTextEntityTag(etag, "user-preferences", "self")
	if err != nil {
		return UserPreferencesRecord{}, ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return UserPreferencesRecord{}, ErrPrecondition
	}
	return coverage.store.UpdateUserPreferences(ctx, actor.User.ID, expectedRevision, patch)
}

// ListViewOverrides 返回租户内全部视图覆盖。
func (coverage *Coverage) ListViewOverrides(ctx context.Context, actor Principal, tenantSlug string) ([]ViewOverrideRecord, error) {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsRead)
	if err != nil {
		return nil, err
	}
	return coverage.store.ListViewOverrides(ctx, membership.TenantID)
}

// PutViewOverride 在 If-Match 下创建或替换一条视图覆盖。
func (coverage *Coverage) PutViewOverride(ctx context.Context, actor Principal, tenantSlug, viewID, etag string, enabled *bool, defaultOptions map[string]any) (ViewOverrideRecord, error) {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return ViewOverrideRecord{}, err
	}
	enabledValue := true
	if enabled != nil {
		enabledValue = *enabled
	}
	options, err := marshalDefaultOptions(defaultOptions)
	if err != nil {
		return ViewOverrideRecord{}, err
	}
	// 未存在覆盖时 etag 可为空（首次创建）；已存在时校验版本号。
	if current, err := coverage.store.GetViewOverride(ctx, membership.TenantID, viewID); err == nil {
		expectedRevision, parseErr := parseTextEntityTag(etag, "view-override", viewID)
		if parseErr != nil {
			return ViewOverrideRecord{}, ErrPrecondition
		}
		if expectedRevision != current.Revision {
			return ViewOverrideRecord{}, ErrPrecondition
		}
	}
	return coverage.store.UpsertViewOverride(ctx, membership.TenantID, viewID, enabledValue, viewOverrideOrdDefault, options)
}

// DeleteViewOverride 在 If-Match 下删除一条视图覆盖。
func (coverage *Coverage) DeleteViewOverride(ctx context.Context, actor Principal, tenantSlug, viewID, etag string) error {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return err
	}
	current, err := coverage.store.GetViewOverride(ctx, membership.TenantID, viewID)
	if err != nil {
		return err
	}
	expectedRevision, err := parseTextEntityTag(etag, "view-override", viewID)
	if err != nil {
		return ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return ErrPrecondition
	}
	return coverage.store.DeleteViewOverride(ctx, membership.TenantID, viewID)
}

// ListTags 返回租户内全部标签。
func (coverage *Coverage) ListTags(ctx context.Context, actor Principal, tenantSlug string) ([]TagRecord, error) {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return nil, err
	}
	return coverage.store.ListTags(ctx, membership.TenantID)
}

// CreateTag 创建一条标签。
func (coverage *Coverage) CreateTag(ctx context.Context, actor Principal, tenantSlug string, input NewTag) (TagRecord, error) {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return TagRecord{}, err
	}
	if err := validateTagInput(input.Name, input.Color); err != nil {
		return TagRecord{}, err
	}
	input.TenantID = membership.TenantID
	input.ID = uuid.NewV7()
	if input.Color == nil {
		input.Color = new(string)
		*input.Color = tagColorDefault
	}
	return coverage.store.CreateTag(ctx, input)
}

// UpdateTag 在 If-Match 下更新一条标签。
func (coverage *Coverage) UpdateTag(ctx context.Context, actor Principal, tenantSlug string, tagID uuid.UUID, etag string, patch TagPatch) (TagRecord, error) {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return TagRecord{}, err
	}
	current, err := coverage.store.GetTag(ctx, membership.TenantID, tagID)
	if err != nil {
		return TagRecord{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "tag", tagID)
	if err != nil {
		return TagRecord{}, ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return TagRecord{}, ErrPrecondition
	}
	if patch.Name != nil {
		if err := validateTagInput(*patch.Name, patch.Color); err != nil {
			return TagRecord{}, err
		}
	}
	return coverage.store.UpdateTag(ctx, membership.TenantID, tagID, expectedRevision, patch)
}

// DeleteTag 在 If-Match 下删除一条标签。
func (coverage *Coverage) DeleteTag(ctx context.Context, actor Principal, tenantSlug string, tagID uuid.UUID, etag string) error {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return err
	}
	current, err := coverage.store.GetTag(ctx, membership.TenantID, tagID)
	if err != nil {
		return err
	}
	expectedRevision, err := parseRevisionETag(etag, "tag", tagID)
	if err != nil {
		return ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return ErrPrecondition
	}
	return coverage.store.DeleteTag(ctx, membership.TenantID, tagID, expectedRevision)
}

// ListServiceComments 返回一个服务下的评论分页。
func (coverage *Coverage) ListServiceComments(ctx context.Context, actor Principal, tenantSlug, serviceSlug string, page, pageSize int) ([]CommentRecord, int64, error) {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	service, err := coverage.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return nil, 0, err
	}
	return coverage.store.ListServiceComments(ctx, membership.TenantID, service.ID, int32(pageSize), int32((page-1)*pageSize))
}

// CreateServiceComment 创建一条服务评论。
func (coverage *Coverage) CreateServiceComment(ctx context.Context, actor Principal, tenantSlug, serviceSlug, body string) (CommentRecord, error) {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return CommentRecord{}, err
	}
	if body == "" || utf8.RuneCountInString(body) > maxServiceCommentBodyRunes {
		return CommentRecord{}, ErrValidation
	}
	service, err := coverage.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return CommentRecord{}, err
	}
	return coverage.store.CreateServiceComment(ctx, membership.TenantID, service.ID, membership.UserID, uuid.NewV7(), body)
}

// CreateTenantExport 创建一条租户导出任务记录并返回 202 投影。
func (coverage *Coverage) CreateTenantExport(ctx context.Context, actor Principal, tenantSlug string) (TenantExportRecord, error) {
	membership, err := coverage.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsRead)
	if err != nil {
		return TenantExportRecord{}, err
	}
	return coverage.store.CreateTenantExport(ctx, membership.TenantID, membership.UserID, uuid.NewV7())
}

func validateTagInput(name string, color *string) error {
	if name == "" || utf8.RuneCountInString(name) > maxTagNameRunes {
		return ErrValidation
	}
	if color != nil && utf8.RuneCountInString(*color) > maxTagColorRunes {
		return ErrValidation
	}
	return nil
}

func marshalDefaultOptions(options map[string]any) ([]byte, error) {
	if options == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(options)
}

func (coverage *Coverage) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	return resolveTenantMembership(ctx, actor, tenantSlug, permission, coverage.identities)
}
