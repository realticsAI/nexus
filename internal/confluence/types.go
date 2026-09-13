package confluence

type Page struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Space   string `json:"space"`
	Body    string `json:"body"`
	Version int    `json:"version"`
	URL     string `json:"url"`
}

type Comment struct {
	ID      string `json:"id"`
	Author  string `json:"author"`
	Body    string `json:"body"`
	Created string `json:"created"`
}

type SearchResult struct {
	Pages []Page `json:"pages"`
	Total int    `json:"total"`
}
