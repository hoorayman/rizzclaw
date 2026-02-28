package tools

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/hoorayman/rizzclaw/internal/llm"
)

var browserHeaders = map[string]string{
	"User-Agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
	"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
	"Accept-Language":           "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6",
	"Accept-Encoding":           "gzip, deflate, br",
	"Connection":                "keep-alive",
	"Upgrade-Insecure-Requests": "1",
	"Sec-Fetch-Dest":            "document",
	"Sec-Fetch-Mode":            "navigate",
	"Sec-Fetch-Site":            "none",
	"Sec-Fetch-User":            "?1",
	"Cache-Control":             "max-age=0",
}

func init() {
	RegisterTool(&ToolDefinition{
		Name:        "web_search",
		Description: "Search the web for information. Returns a list of relevant results with titles, URLs, and snippets. Use this to find current information or research topics.",
		Handler:     WebSearch,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"query": {
					Type:        "string",
					Description: "The search query",
				},
				"count": {
					Type:        "integer",
					Description: "Number of results to return (default: 5, max: 10)",
				},
			},
			Required: []string{"query"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "web_fetch",
		Description: "Fetch and extract content from a URL. Converts HTML to readable text/markdown. Use this to read the full content of a web page.",
		Handler:     WebFetch,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"url": {
					Type:        "string",
					Description: "The URL to fetch",
				},
				"extract_mode": {
					Type:        "string",
					Description: "Extraction mode: 'markdown' or 'text' (default: markdown)",
				},
				"max_chars": {
					Type:        "integer",
					Description: "Maximum characters to return (default: 10000)",
				},
			},
			Required: []string{"url"},
		},
	})
}

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func WebSearch(ctx context.Context, input map[string]any) (string, error) {
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query is required")
	}

	count := 5
	if c, ok := input["count"].(float64); ok {
		count = int(c)
		if count > 10 {
			count = 10
		}
		if count < 1 {
			count = 1
		}
	}

	// Try Brave API first if key is available
	apiKey := os.Getenv("BRAVE_API_KEY")
	if apiKey != "" {
		result, err := webSearchBrave(ctx, query, count, apiKey)
		if err == nil {
			return result, nil
		}
	}

	// Use DuckDuckGo HTML search as default (more reliable than Bing scraping)
	return webSearchDuckDuckGo(ctx, query, count)
}

func webSearchBing(ctx context.Context, query string, count int) (string, error) {
	endpoint := fmt.Sprintf("https://www.bing.com/search?q=%s", url.QueryEscape(query))

	debug := os.Getenv("RIZZ_DEBUG") != ""
	if debug {
		fmt.Printf("[DEBUG] Bing search URL: %s\n", endpoint)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range browserHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		if debug {
			fmt.Printf("[DEBUG] Request failed: %v\n", err)
		}
		return "", fmt.Errorf("failed to fetch search results: %w", err)
	}
	defer resp.Body.Close()

	if debug {
		fmt.Printf("[DEBUG] Response status: %d\n", resp.StatusCode)
		fmt.Printf("[DEBUG] Content-Type: %s\n", resp.Header.Get("Content-Type"))
		fmt.Printf("[DEBUG] Content-Encoding: %s\n", resp.Header.Get("Content-Encoding"))
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Bing returned status %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	encoding := resp.Header.Get("Content-Encoding")
	if debug {
		fmt.Printf("[DEBUG] Content-Encoding: %s\n", encoding)
	}

	switch encoding {
	case "gzip":
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			if debug {
				fmt.Printf("[DEBUG] Gzip reader error: %v\n", err)
			}
			return "", fmt.Errorf("gzip decode error: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	case "br":
		reader = brotli.NewReader(resp.Body)
	case "deflate":
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		if debug {
			fmt.Printf("[DEBUG] Read body error: %v\n", err)
		}
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if debug {
		fmt.Printf("[DEBUG] Body length: %d bytes\n", len(body))
	}

	html := string(body)
	text := htmlToPlainText(html)

	maxLen := 15000
	if len(text) > maxLen {
		text = text[:maxLen] + "\n\n... [truncated]"
	}

	return fmt.Sprintf("搜索关键词: %s\n\n%s", query, text), nil
}

func htmlToPlainText(html string) string {
	re := regexp.MustCompile(`<style[^>]*>[\s\S]*?</style>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<script[^>]*>[\s\S]*?</script>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<nav[^>]*>[\s\S]*?</nav>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<footer[^>]*>[\s\S]*?</footer>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<header[^>]*>[\s\S]*?</header>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<aside[^>]*>[\s\S]*?</aside>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<form[^>]*>[\s\S]*?</form>`)
	html = re.ReplaceAllString(html, "")

	html = strings.ReplaceAll(html, "</title>", " - ")
	html = strings.ReplaceAll(html, "</h1>", "\n\n")
	html = strings.ReplaceAll(html, "</h2>", "\n\n")
	html = strings.ReplaceAll(html, "</h3>", "\n")
	html = strings.ReplaceAll(html, "</h4>", "\n")
	html = strings.ReplaceAll(html, "</h5>", "\n")
	html = strings.ReplaceAll(html, "</h6>", "\n")
	html = strings.ReplaceAll(html, "</p>", "\n")
	html = strings.ReplaceAll(html, "</div>", "\n")
	html = strings.ReplaceAll(html, "</li>", "\n")
	html = strings.ReplaceAll(html, "</a>", " ")
	html = strings.ReplaceAll(html, "<br>", "\n")
	html = strings.ReplaceAll(html, "<br/>", "\n")
	html = strings.ReplaceAll(html, "<br />", "\n")

	re = regexp.MustCompile(`<a[^>]*href="([^"]+)"[^>]*>`)
	html = re.ReplaceAllString(html, "[$1] ")

	re = regexp.MustCompile(`<[^>]+>`)
	html = re.ReplaceAllString(html, "")

	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&apos;", "'")

	re = regexp.MustCompile(`\s+`)
	html = re.ReplaceAllString(html, " ")

	re = regexp.MustCompile(`\n\s*\n`)
	html = re.ReplaceAllString(html, "\n\n")

	return strings.TrimSpace(html)
}

func cleanHTMLTags(s string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	s = re.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.TrimSpace(s)
	return s
}

func webSearchBrave(ctx context.Context, query string, count int, apiKey string) (string, error) {
	endpoint := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d", url.QueryEscape(query), count)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch search results: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("search API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var braveResp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.Unmarshal(body, &braveResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	results := make([]SearchResult, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}

	return formatSearchResults(query, results)
}

func webSearchDuckDuckGo(ctx context.Context, query string, count int) (string, error) {
	// Use DuckDuckGo HTML interface instead of API for better results
	endpoint := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	// Create HTTP client with proxy support
	client, err := createHTTPClientWithProxy()
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP client: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Use simple User-Agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch search results: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("DuckDuckGo returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return extractDuckDuckGoResults(string(body), count, query)
}

// createHTTPClientWithProxy creates an HTTP client with optional proxy support
func createHTTPClientWithProxy() (*http.Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		TLSHandshakeTimeout: 15 * time.Second,
	}

	// Check for proxy environment variables
	proxyURL := os.Getenv("HTTP_PROXY")
	if proxyURL == "" {
		proxyURL = os.Getenv("http_proxy")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("HTTPS_PROXY")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("https_proxy")
	}

	if proxyURL != "" {
		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(parsedURL)
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}

	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}, nil
}

func extractDuckDuckGoResults(html string, count int, query string) (string, error) {
	// Extract result links: <a class="result__a" href="...">Title</a>
	reLink := regexp.MustCompile(`<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	matches := reLink.FindAllStringSubmatch(html, count+5)

	if len(matches) == 0 {
		return fmt.Sprintf("No results found for: %s", query), nil
	}

	// Extract snippets: <a class="result__snippet" ...>Snippet</a>
	reSnippet := regexp.MustCompile(`<a class="result__snippet[^"]*".*?>([\s\S]*?)</a>`)
	snippetMatches := reSnippet.FindAllStringSubmatch(html, count+5)

	var lines []string
	lines = append(lines, fmt.Sprintf("Results for: %s (via DuckDuckGo)", query))

	maxItems := min(len(matches), count)
	for i := 0; i < maxItems; i++ {
		urlStr := matches[i][1]
		title := stripTags(matches[i][2])
		title = strings.TrimSpace(title)

		// Decode DuckDuckGo redirect URL if present
		if strings.Contains(urlStr, "uddg=") {
			if u, err := url.QueryUnescape(urlStr); err == nil {
				idx := strings.Index(u, "uddg=")
				if idx != -1 {
					urlStr = u[idx+5:]
				}
			}
		}

		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, title, urlStr))

		// Add snippet if available
		if i < len(snippetMatches) {
			snippet := stripTags(snippetMatches[i][1])
			snippet = strings.TrimSpace(snippet)
			if snippet != "" {
				lines = append(lines, fmt.Sprintf("   %s", snippet))
			}
		}
	}

	return strings.Join(lines, "\n"), nil
}

func stripTags(content string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	return re.ReplaceAllString(content, "")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func extractTitle(text string) string {
	if idx := strings.Index(text, " - "); idx > 0 {
		return text[:idx]
	}
	if len(text) > 50 {
		return text[:50] + "..."
	}
	return text
}

func formatSearchResults(query string, results []SearchResult) (string, error) {
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d search results for: %s\n\n", len(results), query))

	for i, r := range results {
		output.WriteString(fmt.Sprintf("【%d】%s\n", i+1, r.Title))
		output.WriteString(fmt.Sprintf("链接: %s\n", r.URL))
		if r.Snippet != "" {
			output.WriteString(fmt.Sprintf("摘要: %s\n", r.Snippet))
		}
		output.WriteString("\n")
	}

	return output.String(), nil
}

func WebFetch(ctx context.Context, input map[string]any) (string, error) {
	targetURL, ok := input["url"].(string)
	if !ok || targetURL == "" {
		return "", fmt.Errorf("url is required")
	}

	extractMode := "markdown"
	if m, ok := input["extract_mode"].(string); ok {
		if m == "text" || m == "markdown" {
			extractMode = m
		}
	}

	maxChars := 10000
	if m, ok := input["max_chars"].(float64); ok {
		maxChars = int(m)
		if maxChars > 50000 {
			maxChars = 50000
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	content := string(body)

	content = extractMainContent(content)

	if extractMode == "text" {
		content = htmlToText(content)
	} else {
		content = htmlToMarkdown(content)
	}

	content = cleanContent(content)

	if len(content) > maxChars {
		content = content[:maxChars] + "\n\n... [content truncated]"
	}

	return fmt.Sprintf("Content from: %s\n\n%s", targetURL, content), nil
}

func extractMainContent(html string) string {
	bodyStart := strings.Index(html, "<body")
	if bodyStart != -1 {
		bodyEnd := strings.Index(html[bodyStart:], "</body>")
		if bodyEnd != -1 {
			html = html[bodyStart : bodyStart+bodyEnd+7]
		}
	}

	articleStart := strings.Index(html, "<article")
	if articleStart != -1 {
		articleEnd := strings.Index(html[articleStart:], "</article>")
		if articleEnd != -1 {
			return html[articleStart : articleStart+articleEnd+10]
		}
	}

	mainStart := strings.Index(html, "<main")
	if mainStart != -1 {
		mainEnd := strings.Index(html[mainStart:], "</main>")
		if mainEnd != -1 {
			return html[mainStart : mainStart+mainEnd+7]
		}
	}

	return html
}

func htmlToText(html string) string {
	re := regexp.MustCompile(`<script[^>]*>[\s\S]*?</script>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<style[^>]*>[\s\S]*?</style>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<nav[^>]*>[\s\S]*?</nav>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<footer[^>]*>[\s\S]*?</footer>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<header[^>]*>[\s\S]*?</header>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<[^>]+>`)
	text := re.ReplaceAllString(html, " ")

	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")

	re = regexp.MustCompile(`\s+`)
	text = re.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

func htmlToMarkdown(html string) string {
	html = strings.ReplaceAll(html, "\r\n", "\n")

	re := regexp.MustCompile(`<script[^>]*>[\s\S]*?</script>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<style[^>]*>[\s\S]*?</style>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<nav[^>]*>[\s\S]*?</nav>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<footer[^>]*>[\s\S]*?</footer>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<h1[^>]*>([\s\S]*?)</h1>`)
	html = re.ReplaceAllString(html, "\n# $1\n")

	re = regexp.MustCompile(`<h2[^>]*>([\s\S]*?)</h2>`)
	html = re.ReplaceAllString(html, "\n## $1\n")

	re = regexp.MustCompile(`<h3[^>]*>([\s\S]*?)</h3>`)
	html = re.ReplaceAllString(html, "\n### $1\n")

	re = regexp.MustCompile(`<h4[^>]*>([\s\S]*?)</h4>`)
	html = re.ReplaceAllString(html, "\n#### $1\n")

	re = regexp.MustCompile(`<p[^>]*>([\s\S]*?)</p>`)
	html = re.ReplaceAllString(html, "\n$1\n")

	re = regexp.MustCompile(`<br\s*/?>`)
	html = re.ReplaceAllString(html, "\n")

	re = regexp.MustCompile(`<li[^>]*>([\s\S]*?)</li>`)
	html = re.ReplaceAllString(html, "- $1\n")

	re = regexp.MustCompile(`<ul[^>]*>([\s\S]*?)</ul>`)
	html = re.ReplaceAllString(html, "\n$1\n")

	re = regexp.MustCompile(`<ol[^>]*>([\s\S]*?)</ol>`)
	html = re.ReplaceAllString(html, "\n$1\n")

	re = regexp.MustCompile(`<strong[^>]*>([\s\S]*?)</strong>`)
	html = re.ReplaceAllString(html, "**$1**")

	re = regexp.MustCompile(`<b[^>]*>([\s\S]*?)</b>`)
	html = re.ReplaceAllString(html, "**$1**")

	re = regexp.MustCompile(`<em[^>]*>([\s\S]*?)</em>`)
	html = re.ReplaceAllString(html, "*$1*")

	re = regexp.MustCompile(`<i[^>]*>([\s\S]*?)</i>`)
	html = re.ReplaceAllString(html, "*$1*")

	re = regexp.MustCompile(`<code[^>]*>([\s\S]*?)</code>`)
	html = re.ReplaceAllString(html, "`$1`")

	re = regexp.MustCompile(`<pre[^>]*>([\s\S]*?)</pre>`)
	html = re.ReplaceAllString(html, "\n```\n$1\n```\n")

	re = regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>([\s\S]*?)</a>`)
	html = re.ReplaceAllString(html, "[$2]($1)")

	re = regexp.MustCompile(`<img[^>]*alt="([^"]*)"[^>]*src="([^"]*)"[^>]*>`)
	html = re.ReplaceAllString(html, "![$1]($2)")

	re = regexp.MustCompile(`<[^>]+>`)
	html = re.ReplaceAllString(html, "")

	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")

	return html
}

func cleanContent(content string) string {
	re := regexp.MustCompile(`\n{3,}`)
	content = re.ReplaceAllString(content, "\n\n")

	re = regexp.MustCompile(`[ \t]+`)
	content = re.ReplaceAllString(content, " ")

	return strings.TrimSpace(content)
}
