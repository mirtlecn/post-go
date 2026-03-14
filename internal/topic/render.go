package topic

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"post-go/internal/convert"
)

// Item describes a topic entry needed to build the index page.
type Item struct {
	Path      string
	FullPath  string
	Type      string
	Title     string
	UpdatedAt time.Time
}

// BuildIndexMarkdown renders the topic index as Markdown grouped by year.
func BuildIndexMarkdown(topicName string, items []Item) string {
	var builder strings.Builder
	builder.WriteString("# ")
	builder.WriteString(topicName)
	builder.WriteString("\n")

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

	currentYear := 0
	for _, item := range sorted {
		year := item.UpdatedAt.Year()
		if year != currentYear {
			builder.WriteString("\n## ")
			builder.WriteString(fmt.Sprintf("%d", year))
			builder.WriteString("\n\n")
			currentYear = year
		}

		builder.WriteString("- [")
		builder.WriteString(displayTitle(topicName, item))
		builder.WriteString("](./")
		builder.WriteString(item.Path)
		builder.WriteString(")")
		if mark := typeMark(item.Type); mark != "" {
			builder.WriteString(" ")
			builder.WriteString(mark)
		}
		builder.WriteString(" · ")
		builder.WriteString(item.UpdatedAt.Format("01-02"))
		builder.WriteString("\n")
	}

	return builder.String()
}

// RenderIndexHTML converts a topic index Markdown document into HTML.
func RenderIndexHTML(topicName string, items []Item) (string, error) {
	return convert.ConvertMarkdownToHTMLWithOptions(
		BuildIndexMarkdown(topicName, items),
		convert.MarkdownOptions{PageTitle: topicName},
	)
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
	case "text":
		return "☰"
	case "file":
		return "◫"
	default:
		return ""
	}
}
