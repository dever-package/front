package article

type ImportURLInput struct {
	URL       string `json:"url"`
	MaxImages int    `json:"max_images"`
	Cookie    string `json:"-"`
}

type ImportedArticle struct {
	Title     string         `json:"title"`
	HTML      string         `json:"html"`
	Assets    []ArticleAsset `json:"assets"`
	SourceURL string         `json:"source_url"`
	Site      string         `json:"site"`
}

type ArticleAsset struct {
	Kind      string `json:"kind"`
	SourceURL string `json:"sourceUrl"`
	Name      string `json:"name"`
}

type fetchedArticlePage struct {
	URL        string
	SourceHTML string
}
