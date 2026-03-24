package parser

import (
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/emre-safa/crawler/internal/types"
)

// Parse processes raw HTML and extracts the page title, visible body text, and links.
// baseURL is used to resolve relative URLs.
func Parse(rawHTML string, item types.CrawlItem) types.PageData {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return types.PageData{
			URL:       item.URL,
			OriginURL: item.OriginURL,
			Depth:     item.Depth,
			FetchedAt: time.Now(),
		}
	}

	var title string
	var bodyParts []string
	var links []string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		// Skip script, style, noscript content
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "iframe":
				return
			}
		}

		// Extract title
		if n.Type == html.ElementNode && n.Data == "title" {
			title = extractText(n)
		}

		// Extract links
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					resolved := resolveURL(item.URL, attr.Val)
					if resolved != "" && isCrawlable(resolved) {
						links = append(links, resolved)
					}
				}
			}
		}

		// Collect visible text
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				bodyParts = append(bodyParts, text)
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Deduplicate links
	links = dedup(links)

	return types.PageData{
		URL:       item.URL,
		OriginURL: item.OriginURL,
		Depth:     item.Depth,
		Title:     title,
		Body:      strings.Join(bodyParts, " "),
		Links:     links,
		FetchedAt: time.Now(),
	}
}

// extractText returns all text content within a node, concatenated.
func extractText(n *html.Node) string {
	var parts []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			parts = append(parts, strings.TrimSpace(n.Data))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(parts, " ")
}

// resolveURL converts a potentially relative href to an absolute URL.
func resolveURL(baseURL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return ""
	}
	// Skip non-HTTP schemes
	if strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:") || strings.HasPrefix(href, "data:") {
		return ""
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(ref)
	resolved.Fragment = "" // strip fragment
	return resolved.String()
}

// isCrawlable checks if a URL uses HTTP(S).
func isCrawlable(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// dedup removes duplicate strings while preserving order.
func dedup(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, s := range items {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
