package topic

import (
	"html"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"post-go/internal/convert"
)

var displayTimeLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// Item describes a topic entry needed to build the index page.
type Item struct {
	Path      string
	FullPath  string
	Type      string
	Title     string
	UpdatedAt time.Time
}

// BuildIndexMarkdown renders the topic index as Markdown.
func BuildIndexMarkdown(topicPath, topicTitle string, items []Item) string {
	var builder strings.Builder
	builder.WriteString("<div style=\"font-size: 1.3em; font-weight: bold\">")
	builder.WriteString(html.EscapeString(convert.CapitalizeTopicLabel(topicTitle)))
	builder.WriteString("</div>\n\n")
	builder.WriteString("<span style=\"color: #666;\">Home</span>")
	builder.WriteString("\n\n\n\n\n\n")

	if len(items) == 0 {
		return builder.String()
	}

	sorted := append([]Item(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].UpdatedAt.Equal(sorted[j].UpdatedAt) {
			return sorted[i].Path < sorted[j].Path
		}
		return sorted[i].UpdatedAt.After(sorted[j].UpdatedAt)
	})

	builder.WriteString("\n")
	if shouldGroupByDisplayYear(sorted) {
		writeGroupedIndexItems(&builder, topicPath, sorted)
		return builder.String()
	}

	for _, item := range sorted {
		writeIndexItem(&builder, topicPath, item, " ", item.UpdatedAt.In(displayTimeLocation).Format("2006-01-02"))
	}

	return builder.String()
}

// RenderIndexHTML converts a topic index Markdown document into HTML.
func RenderIndexHTML(topicPath, topicTitle string, items []Item) (string, error) {
	return convert.ConvertMarkdownToHTMLWithOptions(
		BuildIndexMarkdown(topicPath, topicTitle, items),
		convert.MarkdownOptions{PageTitle: topicTitle},
	)
}

func buildTopicItemHref(topicPath, itemPath string) string {
	return path.Join("/", topicPath, itemPath)
}

func formatMarkdownLinkDestination(destination string) string {
	return "<" + destination + ">"
}

func shouldGroupByDisplayYear(items []Item) bool {
	if len(items) <= 10 {
		return false
	}
	firstYear := items[0].UpdatedAt.In(displayTimeLocation).Year()
	for _, item := range items[1:] {
		if item.UpdatedAt.In(displayTimeLocation).Year() != firstYear {
			return true
		}
	}
	return false
}

func writeGroupedIndexItems(builder *strings.Builder, topicPath string, items []Item) {
	currentYear := 0
	for _, item := range items {
		displayTime := item.UpdatedAt.In(displayTimeLocation)
		if year := displayTime.Year(); year != currentYear {
			if currentYear != 0 {
				builder.WriteString("\n")
			}
			currentYear = year
			builder.WriteString("## ")
			builder.WriteString(strconv.Itoa(year))
			builder.WriteString("\n")
		}
		writeIndexItem(builder, topicPath, item, " · ", displayTime.Format("01-02"))
	}
}

func writeIndexItem(builder *strings.Builder, topicPath string, item Item, datePrefix, date string) {
	builder.WriteString("- [")
	builder.WriteString(displayTitle(topicPath, item))
	builder.WriteString("](")
	builder.WriteString(formatMarkdownLinkDestination(buildTopicItemHref(topicPath, item.Path)))
	builder.WriteString(")")
	if mark := typeMark(item.Type); mark != "" {
		builder.WriteString(" ")
		builder.WriteString(mark)
	}
	builder.WriteString(datePrefix)
	builder.WriteString(date)
	builder.WriteString("\n")
}

func displayTitle(topicName string, item Item) string {
	if item.Title != "" {
		return item.Title
	}
	fullPath := item.FullPath
	if fullPath == "" {
		fullPath = topicName + "/" + item.Path
	}
	prefix := topicName + "/"
	if strings.HasPrefix(fullPath, prefix) {
		return strings.TrimPrefix(fullPath, prefix)
	}
	if item.Path != "" {
		return item.Path
	}
	return fullPath
}

func typeMark(itemType string) string {
	switch itemType {
	case "url":
		return "↗"
	case "text", "qrcode":
		return "☰"
	case "file":
		return "◫"
	default:
		return ""
	}
}
