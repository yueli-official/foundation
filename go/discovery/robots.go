package discovery

import (
	"context"
	"strings"
)

func (module *Module) publishRobots(
	ctx context.Context,
	plan RobotsPlan,
	transaction PublicationWriter,
) (Artifact, error) {
	route, err := normalizeArtifactRoute(plan.Route, "robots.txt", "robots.route")
	if err != nil {
		return Artifact{}, err
	}
	groups := plan.Groups
	if len(groups) == 0 {
		groups = []RobotsGroup{{UserAgents: []string{"*"}, Allow: []string{"/"}}}
	}
	var builder strings.Builder
	for groupIndex, group := range groups {
		if len(group.UserAgents) == 0 {
			return Artifact{}, failure(ErrorContract, "robots_user_agent_required", "robots.groups", "group %d has no user-agent", groupIndex)
		}
		for _, agent := range group.UserAgents {
			if strings.TrimSpace(agent) == "" || invalidRobotsValue(agent) {
				return Artifact{}, failure(ErrorContract, "invalid_robots_value", "robots.groups.userAgents", "contains a line break or control character")
			}
			builder.WriteString("User-agent: ")
			builder.WriteString(strings.TrimSpace(agent))
			builder.WriteByte('\n')
		}
		for _, rule := range group.Allow {
			if err := writeRobotsRule(&builder, "Allow", rule); err != nil {
				return Artifact{}, err
			}
		}
		for _, rule := range group.Disallow {
			if err := writeRobotsRule(&builder, "Disallow", rule); err != nil {
				return Artifact{}, err
			}
		}
		builder.WriteByte('\n')
	}
	for index, sitemap := range plan.Sitemaps {
		location, err := module.absoluteURL(sitemap, "robots.sitemaps")
		if err != nil {
			return Artifact{}, err
		}
		if index > 0 && location == "" {
			continue
		}
		builder.WriteString("Sitemap: ")
		builder.WriteString(location)
		builder.WriteByte('\n')
	}
	if builder.Len() > module.limits.MaxRobotsBytes {
		return Artifact{}, failure(ErrorCapacity, "robots_byte_limit", "robots", "exceeds %d bytes", module.limits.MaxRobotsBytes)
	}
	writer, err := createArtifact(ctx, transaction, route, "text/plain; charset=utf-8")
	if err != nil {
		return Artifact{}, err
	}
	if err := writeString(writer, builder.String()); err != nil {
		return Artifact{}, err
	}
	if err := writer.Close(); err != nil {
		return Artifact{}, targetError("target_close_failed", true, err)
	}
	return writer.artifact(route, "text/plain; charset=utf-8"), nil
}

func writeRobotsRule(builder *strings.Builder, directive, value string) error {
	value = strings.TrimSpace(value)
	if value != "" && !strings.HasPrefix(value, "/") && value != "*" {
		return failure(ErrorContract, "invalid_robots_rule", "robots.groups", "%s rule must be empty, '*' or start with '/'", directive)
	}
	if invalidRobotsValue(value) {
		return failure(ErrorContract, "invalid_robots_value", "robots.groups", "contains a line break or control character")
	}
	builder.WriteString(directive)
	builder.WriteString(": ")
	builder.WriteString(value)
	builder.WriteByte('\n')
	return nil
}

func invalidRobotsValue(value string) bool {
	return strings.ContainsAny(value, "\r\n\x00")
}
