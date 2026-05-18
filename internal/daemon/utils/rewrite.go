package utils

import (
	"fmt"
	"regexp"
	"strings"

	serverembed "github.com/pedroborgesdev/tunnerse-cli/internal/daemon/embed"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/logger"
)

var (
	attributePathPattern = regexp.MustCompile(`(?i)\b(href|src|action)=(["'])(/[^"'\s>]*)`)
	modulePathPattern    = regexp.MustCompile(`(?m)(\b(?:from|import)\s*(?:\(\s*)?["'])(/[^"']*)`)
	cssURLPathPattern    = regexp.MustCompile(`(?i)(url\(\s*["']?)(/[^"')\s]*)`)
	bodyTagPattern       = regexp.MustCompile(`(?i)<body\b[^>]*>`)
	htmlTagPattern       = regexp.MustCompile(`(?i)<html\b[^>]*>`)
)

func RewriteAbsolutePaths(html []byte, tunnelName string) []byte {
	content := string(html)
	prefix := fmt.Sprintf("/%s", tunnelName)

	content = rewriteAttributePaths(content, prefix)
	content = rewriteModulePaths(content, prefix)
	content = rewriteCSSURLPaths(content, prefix)

	return []byte(content)
}

func RewriteAbsolutePathsForContentType(body []byte, tunnelName, contentType string) []byte {
	if shouldRewriteContentType(contentType) || looksLikeHTML(body) {
		rewritten := RewriteAbsolutePaths(body, tunnelName)
		if shouldInjectTunnerseTunnelHeader(rewritten, contentType) {
			return InjectTunnerseTunnelHeader(rewritten)
		}
		return rewritten
	}
	return body
}

func shouldRewriteContentType(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "text/html") ||
		strings.Contains(contentType, "javascript") ||
		strings.Contains(contentType, "ecmascript") ||
		strings.Contains(contentType, "text/css")
}

func isHTMLContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/html")
}

func InjectTunnerseTunnelHeaderForContentType(body []byte, contentType string) []byte {
	if shouldInjectTunnerseTunnelHeader(body, contentType) {
		return InjectTunnerseTunnelHeader(body)
	}
	return body
}

func shouldInjectTunnerseTunnelHeader(body []byte, contentType string) bool {
	return isHTMLContentType(contentType) || looksLikeHTML(body)
}

func looksLikeHTML(body []byte) bool {
	content := strings.TrimSpace(string(body))
	content = strings.ToLower(content)
	return strings.HasPrefix(content, "<!doctype html") ||
		strings.HasPrefix(content, "<html") ||
		bodyTagPattern.FindStringIndex(content) != nil
}

func InjectTunnerseTunnelHeader(body []byte) []byte {
	html := string(body)
	if strings.Contains(html, `id="tunnerse-tunnel-header"`) {
		return body
	}

	header := serverembed.HeaderHTML()
	if match := bodyTagPattern.FindStringIndex(html); match != nil {
		logger.Log("INFO", "Tunnerse tunnel header inserted into HTML", []logger.LogDetail{{Key: "position", Value: "body"}})
		html = html[:match[1]] + "\n" + header + html[match[1]:]
		return []byte(html)
	}
	if match := htmlTagPattern.FindStringIndex(html); match != nil {
		logger.Log("INFO", "Tunnerse tunnel header inserted into HTML", []logger.LogDetail{{Key: "position", Value: "html"}})
		html = html[:match[1]] + "\n" + header + html[match[1]:]
		return []byte(html)
	}
	logger.Log("INFO", "Tunnerse tunnel header inserted into HTML", []logger.LogDetail{{Key: "position", Value: "document-start"}})
	return []byte(header + "\n" + html)
}

func rewriteAttributePaths(content, prefix string) string {
	return attributePathPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := attributePathPattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return parts[1] + "=" + parts[2] + prefixPath(parts[3], prefix)
	})
}

func rewriteModulePaths(content, prefix string) string {
	return modulePathPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := modulePathPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		return parts[1] + prefixPath(parts[2], prefix)
	})
}

func rewriteCSSURLPaths(content, prefix string) string {
	return cssURLPathPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := cssURLPathPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		return parts[1] + prefixPath(parts[2], prefix)
	})
}

func prefixPath(path, prefix string) string {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return path
	}
	if path == prefix || strings.HasPrefix(path, prefix+"/") {
		return path
	}
	return prefix + path
}

func InjectBaseHref(body []byte, tunnelName string) []byte {
	html := string(body)
	baseTag := fmt.Sprintf(`<base href="/%s/">`, tunnelName)
	html = strings.Replace(html, "<head>", "<head>\n"+baseTag, 1)
	return []byte(html)
}
