package render

import (
	"encoding/json"
	"html/template"
)

type pageEnvelope struct {
	Page     pageMeta       `json:"page"`
	Data     map[string]any `json:"data"`
	Template templateConfig `json:"template"`
}

type pageMeta struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Render      string `json:"render"`
}

type templateConfig struct {
	Route  string `json:"route"`
	Layout string `json:"layout"`
	View   string `json:"view"`
}

type TemplateRoute struct {
	SiteKey  string
	PageName string
	Path     string
	Route    string
	Config   templateConfig
	Page     pageMeta
	Content  []byte
	Params   []string
	Pattern  string
}

type RenderContext struct {
	Page  pageMeta
	Data  map[string]any
	SEO   SEO
	Route map[string]any
	Query map[string]any
	Site  SiteContext
}

type SEO struct {
	Title       string
	Description string
	Image       string
	Canonical   string
}

type SiteContext struct {
	Key       string
	Path      string
	Page      string
	API       string
	Name      string
	AssetBase string
}

type templateExecutor struct {
	view   string
	layout string
	tpl    *template.Template
}

type rawSchema struct {
	Page     json.RawMessage `json:"page"`
	Layout   json.RawMessage `json:"layout"`
	Nodes    json.RawMessage `json:"nodes"`
	Data     json.RawMessage `json:"data"`
	State    json.RawMessage `json:"state"`
	Action   json.RawMessage `json:"action"`
	Template json.RawMessage `json:"template"`
}
