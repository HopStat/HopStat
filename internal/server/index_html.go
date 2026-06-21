package server

import (
	"bytes"
	"encoding/json"
	"html"

	"github.com/HopStat/HopStat/internal/sitecache"
)

const indexBootstrapMarker = "<!-- hopstat:bootstrap -->"

var indexHTMLMarshal = json.Marshal

func injectIndexHTML(indexHTML []byte) []byte {
	settings := sitecache.AllSettings()
	if len(settings) == 0 {
		return indexHTML
	}

	siteName := settings["site_name"]
	if siteName == "" {
		siteName = "Looking Glass"
	}
	headerColor := settings["header_color"]
	if headerColor == "" {
		headerColor = "#1e293b"
	}

	out := bytes.Replace(
		indexHTML,
		[]byte("<title>Looking Glass</title>"),
		[]byte("<title>"+html.EscapeString(siteName)+"</title>"),
		1,
	)

	payload, err := indexHTMLMarshal(map[string]string{
		"header_color":     headerColor,
		"site_name":        siteName,
		"site_description": settings["site_description"],
	})
	if err != nil {
		return out
	}

	inject := []byte("<script>window.__HOPSTAT_BOOTSTRAP__=" + string(payload) + ";</script>\n    ")
	out = bytes.Replace(out, []byte(indexBootstrapMarker), inject, 1)
	return out
}
