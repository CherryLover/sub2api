package service

// normalizeGroupModelsListConfig 兼容旧调用点：委托给白名单归一化。
func normalizeGroupModelsListConfig(cfg GroupModelAllowlist) GroupModelAllowlist {
	out, err := normalizeGroupModelAllowlist(cfg)
	if err != nil {
		return GroupModelAllowlist{Enabled: false}
	}
	return out
}

// CustomModelsListEnabled 兼容旧调用点。开启白名单即视为启用。
func (g *Group) CustomModelsListEnabled() bool {
	return g.ModelAllowlistEnabled()
}
