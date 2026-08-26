package service

import "context"

// GitHubReleaseClient 获取 GitHub release 信息的接口。
//
// 应用内的自更新（版本检查/在线升级/回滚）已整套移除，本接口现在只服务于
// OpenAICodexVersionSyncService：读取 openai/codex 的 release 以跟随官方客户端
// 版本号。因此只保留读取 release 元数据的两个方法，不再提供二进制下载与
// 校验和获取。
type GitHubReleaseClient interface {
	FetchLatestRelease(ctx context.Context, repo string) (*GitHubRelease, error)
	FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*GitHubRelease, error)
}

// GitHubRelease 是 GitHub release API 的响应结构。
// 只声明消费方真正读取的字段：release 列表页体积可达 10MB 级别，其中绝大部分是
// assets，跟随版本号用不到，不解码即可省下这部分开销。
type GitHubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}
