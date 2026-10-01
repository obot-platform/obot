package subjectgroups

import (
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func Sets(user kuser.Info) (authProvider, obot map[string]struct{}) {
	if user == nil {
		return nil, nil
	}

	extra := user.GetExtra()
	authProvider = make(map[string]struct{}, len(extra["auth_provider_groups"]))
	for _, group := range extra["auth_provider_groups"] {
		authProvider[group] = struct{}{}
	}
	obot = make(map[string]struct{}, len(extra["obot_groups"]))
	for _, group := range extra["obot_groups"] {
		obot[group] = struct{}{}
	}
	return authProvider, obot
}
