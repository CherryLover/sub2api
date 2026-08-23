package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 登录入口 / 默认首页的后台读写。
//
// 这三项能在后台改，也就意味着管理员点几下就能让登录页再也打不开、而服务照常运行。
// 本文件里的每一步都是冲着这个脚枪去的：
//   - 保存前校验，"隐藏但路径为空/非法"直接拒绝，不允许落库；
//   - 本地配置文件显式设置时锁定该项，后台只读（改配置文件重启是破窗通道）；
//   - 生效值（而不是数据库原始值）回给后台，让界面能回显真正会用的登录 URL。

// webEntryUpdatePlan 是本次保存之后这三项的最终取值（已归一化、已校验）。
type webEntryUpdatePlan struct {
	LoginEntryPublic bool
	LoginEntryPath   string
	DefaultHomePath  string
}

// webEntrySettingsToDTO 把三层合并后的生效值填进管理端 payload。
func webEntrySettingsToDTO(entry service.WebEntrySettings, payload *dto.SystemSettings) {
	payload.LoginEntryPublic = entry.LoginEntryPublic
	payload.LoginEntryPath = entry.LoginEntryPath
	payload.DefaultHomePath = entry.DefaultHomePath
	payload.LoginEntryLockedByConfig = entry.LoginEntryLockedByConfig
	payload.DefaultHomePathLockedByConfig = entry.DefaultHomeLockedByConfig
}

// planWebEntryUpdate 合并请求、数据库现值与本地配置锁定状态，并做保存前校验。
//
// 返回 false 表示已经写过错误响应，调用方直接 return。
// 被本地配置锁定的项会被塞进 omitted：它们不写库，请求里带的值只用于"是不是想改锁定项"
// 的判断和校验回显。
func (h *SettingHandler) planWebEntryUpdate(
	c *gin.Context,
	req UpdateSettingsRequest,
	previous *service.SystemSettings,
	omitted service.OmittedSettingKeys,
) (webEntryUpdatePlan, bool) {
	effective := h.settingService.ResolveWebEntry(c.Request.Context())

	plan := webEntryUpdatePlan{
		LoginEntryPublic: previous.LoginEntryPublic,
		LoginEntryPath:   previous.LoginEntryPath,
		DefaultHomePath:  previous.DefaultHomePath,
	}

	loginTouched := req.LoginEntryPublic != nil || req.LoginEntryPath != nil
	switch {
	case effective.LoginEntryLockedByConfig:
		// 锁定：数据库值原封不动，界面上显示的也是配置文件里的那份。请求如果带了
		// 不一样的值，说明有人在对着一个改不动的开关按保存——直接告诉他为什么。
		omitted[service.SettingKeyWebLoginEntryPublic] = struct{}{}
		omitted[service.SettingKeyWebLoginEntryPath] = struct{}{}
		plan.LoginEntryPublic = effective.LoginEntryPublic
		plan.LoginEntryPath = effective.LoginEntryPath
		if loginTouched && webEntryLoginRequestDiffers(req, effective) {
			response.BadRequest(c, "the login entry is pinned by the local config file (web.login_entry_public / web.login_entry_path); edit the config file and restart to change it")
			return webEntryUpdatePlan{}, false
		}
	case loginTouched:
		if req.LoginEntryPublic != nil {
			plan.LoginEntryPublic = *req.LoginEntryPublic
		}
		if req.LoginEntryPath != nil {
			plan.LoginEntryPath = *req.LoginEntryPath
		}
	default:
		// 请求没提这两项：保持库里现值，别让不带新字段的旧客户端把登录入口重置掉。
		omitted[service.SettingKeyWebLoginEntryPublic] = struct{}{}
		omitted[service.SettingKeyWebLoginEntryPath] = struct{}{}
		// 后续校验用生效值而不是数据库原始值：数据库里万一存着一份坏状态
		// （隐藏但路径不可用，解析阶段已 fail-open 成公开），跟着用原始值会让
		// 这条坏数据把整个设置页的保存都卡死——改任何一项都被这里 400 掉。
		plan.LoginEntryPublic = effective.LoginEntryPublic
		plan.LoginEntryPath = effective.LoginEntryPath
	}

	switch {
	case effective.DefaultHomeLockedByConfig:
		omitted[service.SettingKeyWebDefaultHomePath] = struct{}{}
		plan.DefaultHomePath = effective.DefaultHomePath
		if req.DefaultHomePath != nil && service.NormalizeWebEntryPath(*req.DefaultHomePath) != effective.DefaultHomePath {
			response.BadRequest(c, "the default home page is pinned by the local config file (web.default_home_path); edit the config file and restart to change it")
			return webEntryUpdatePlan{}, false
		}
	case req.DefaultHomePath != nil:
		plan.DefaultHomePath = *req.DefaultHomePath
	default:
		omitted[service.SettingKeyWebDefaultHomePath] = struct{}{}
		// 同上：生效值已经过白名单与死循环防护，拿它做校验基线才不会被坏数据卡死。
		plan.DefaultHomePath = effective.DefaultHomePath
	}

	normalizedPath, normalizedHome, err := service.NormalizeAndValidateWebEntryInput(
		plan.LoginEntryPublic, plan.LoginEntryPath, plan.DefaultHomePath)
	if err != nil {
		response.BadRequest(c, err.Error())
		return webEntryUpdatePlan{}, false
	}
	plan.LoginEntryPath = normalizedPath
	plan.DefaultHomePath = normalizedHome
	return plan, true
}

// webEntryLoginRequestDiffers 判断请求里的登录入口值是否与当前生效值不同。
func webEntryLoginRequestDiffers(req UpdateSettingsRequest, effective service.WebEntrySettings) bool {
	if req.LoginEntryPublic != nil && *req.LoginEntryPublic != effective.LoginEntryPublic {
		return true
	}
	if req.LoginEntryPath != nil && service.NormalizeWebEntryPath(*req.LoginEntryPath) != effective.LoginEntryPath {
		return true
	}
	return false
}
