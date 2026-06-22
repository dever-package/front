package article

import (
	"strings"

	nethtml "golang.org/x/net/html"
)

func parseHTMLDocument(source string) (*nethtml.Node, error) {
	return nethtml.Parse(strings.NewReader(source))
}

func findFirstElement(root *nethtml.Node, match func(*nethtml.Node) bool) *nethtml.Node {
	if root == nil {
		return nil
	}
	if root.Type == nethtml.ElementNode && match(root) {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstElement(child, match); found != nil {
			return found
		}
	}
	return nil
}

func findAllElements(root *nethtml.Node, match func(*nethtml.Node) bool, items *[]*nethtml.Node) {
	if root == nil {
		return
	}
	if root.Type == nethtml.ElementNode && match(root) {
		*items = append(*items, root)
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		findAllElements(child, match, items)
	}
}

func findElementByID(root *nethtml.Node, id string) *nethtml.Node {
	id = strings.TrimSpace(id)
	return findFirstElement(root, func(node *nethtml.Node) bool {
		return attrValue(node, "id") == id
	})
}

func findFirstElementByTag(root *nethtml.Node, tags ...string) *nethtml.Node {
	allowed := map[string]struct{}{}
	for _, tag := range tags {
		allowed[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
	}
	return findFirstElement(root, func(node *nethtml.Node) bool {
		_, ok := allowed[strings.ToLower(node.Data)]
		return ok
	})
}

func attrValue(node *nethtml.Node, key string) string {
	if node == nil {
		return ""
	}
	key = strings.ToLower(strings.TrimSpace(key))
	for _, attr := range node.Attr {
		if strings.ToLower(attr.Key) == key {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func hasClassOrIDToken(node *nethtml.Node, tokens ...string) bool {
	value := strings.ToLower(attrValue(node, "id") + " " + attrValue(node, "class"))
	for _, token := range tokens {
		if strings.Contains(value, strings.ToLower(token)) {
			return true
		}
	}
	return false
}

func nodeText(node *nethtml.Node) string {
	if node == nil {
		return ""
	}
	var builder strings.Builder
	appendNodeText(&builder, node)
	return strings.Join(strings.Fields(builder.String()), " ")
}

func appendNodeText(builder *strings.Builder, node *nethtml.Node) {
	if node.Type == nethtml.TextNode {
		builder.WriteString(node.Data)
		builder.WriteByte(' ')
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendNodeText(builder, child)
	}
}

func firstMetaContent(root *nethtml.Node, names ...string) string {
	for _, name := range names {
		target := strings.ToLower(strings.TrimSpace(name))
		if target == "" {
			continue
		}
		if found := findFirstElement(root, func(node *nethtml.Node) bool {
			if strings.ToLower(node.Data) != "meta" {
				return false
			}
			return strings.ToLower(attrValue(node, "property")) == target ||
				strings.ToLower(attrValue(node, "name")) == target
		}); found != nil {
			if content := attrValue(found, "content"); content != "" {
				return cleanTitle(content)
			}
		}
	}
	return ""
}

func resolveDocumentTitle(root *nethtml.Node) string {
	if title := firstMetaContent(root, "og:title", "twitter:title", "article:title"); title != "" {
		return title
	}
	if h1 := findFirstElementByTag(root, "h1"); h1 != nil {
		if title := cleanTitle(nodeText(h1)); title != "" {
			return title
		}
	}
	if titleNode := findFirstElementByTag(root, "title"); titleNode != nil {
		return cleanTitle(nodeText(titleNode))
	}
	return ""
}

func cleanTitle(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	for _, separator := range []string{" - ", " _ ", " | "} {
		if index := strings.LastIndex(value, separator); index > 8 {
			return strings.TrimSpace(value[:index])
		}
	}
	return value
}
