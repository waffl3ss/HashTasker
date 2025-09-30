package handlers

import (
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/russross/blackfriday/v2"
)

type ContentHandler struct{}

func NewContentHandler() *ContentHandler {
	return &ContentHandler{}
}

// TroubleshootingPage serves the troubleshooting guide page
func (h *ContentHandler) TroubleshootingPage(c *gin.Context) {
	content := h.readMarkdownFile("troubleshooting.md")

	user, exists := c.Get("user")
	if !exists {
		c.Redirect(http.StatusTemporaryRedirect, "/login")
		return
	}

	c.HTML(http.StatusOK, "base", gin.H{
		"template": "troubleshooting",
		"title":    "Troubleshooting Guide",
		"content":  content,
		"user":     user,
	})
}

// ChangelogPage serves the changelog page
func (h *ContentHandler) ChangelogPage(c *gin.Context) {
	content := h.readMarkdownFile("changelog.md")

	user, exists := c.Get("user")
	if !exists {
		c.Redirect(http.StatusTemporaryRedirect, "/login")
		return
	}

	c.HTML(http.StatusOK, "base", gin.H{
		"template": "changelog",
		"title":    "Changelog",
		"content":  content,
		"user":     user,
	})
}

// readMarkdownFile reads a markdown file from the current directory and converts it to HTML
func (h *ContentHandler) readMarkdownFile(filename string) template.HTML {
	// Get the executable directory
	execPath, err := os.Executable()
	if err != nil {
		return template.HTML("<p class=\"text-danger\">Error: Could not determine executable path</p>")
	}

	execDir := filepath.Dir(execPath)
	filePath := filepath.Join(execDir, filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return template.HTML("<div class=\"alert alert-warning\">" +
			"<h5>File Not Found</h5>" +
			"<p>The file <code>" + filename + "</code> was not found in the application directory.</p>" +
			"<p>Please create this file in the same directory as the HashTasker binary:</p>" +
			"<pre><code>" + filePath + "</code></pre>" +
			"</div>")
	}

	// Read the file
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return template.HTML("<p class=\"text-danger\">Error reading file: " + err.Error() + "</p>")
	}

	// Convert markdown to HTML
	htmlContent := blackfriday.Run(data)

	return template.HTML(htmlContent)
}