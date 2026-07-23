package discovery

import (
	"fmt"
	"strings"
	"time"
)

type subjectSchemaFacts struct {
	image       *Image
	publishedAt *time.Time
	modifiedAt  *time.Time
	authors     []Person
	section     string
	tags        []string
	product     *Product
}

func (module *Module) subjectFacts(subject Subject) (
	title string,
	description string,
	image *Image,
	modifiedAt *time.Time,
	ogType string,
	schemaType string,
	facts subjectSchemaFacts,
	err error,
) {
	count := 0
	for _, present := range []bool{
		subject.WebPage != nil,
		subject.Article != nil,
		subject.Collection != nil,
		subject.Product != nil,
	} {
		if present {
			count++
		}
	}
	if count != 1 {
		err = failure(ErrorContract, "subject_variant_required", "subject", "must contain exactly one typed variant")
		return
	}
	switch subject.Kind {
	case SubjectWebPage:
		if subject.WebPage == nil {
			err = failure(ErrorContract, "subject_kind_mismatch", "subject.kind", "does not match webPage")
			return
		}
		title, description, image = subject.WebPage.Title, subject.WebPage.Description, subject.WebPage.Image
		modifiedAt = subject.WebPage.ModifiedAt
		ogType, schemaType = "website", "WebPage"
		facts = subjectSchemaFacts{
			image: image, publishedAt: subject.WebPage.PublishedAt,
			modifiedAt: modifiedAt,
		}
	case SubjectArticle:
		if subject.Article == nil {
			err = failure(ErrorContract, "subject_kind_mismatch", "subject.kind", "does not match article")
			return
		}
		if subject.Article.PublishedAt.IsZero() {
			err = failure(ErrorContract, "published_at_required", "subject.article.publishedAt", "is required")
			return
		}
		title, description, image = subject.Article.Title, subject.Article.Description, subject.Article.Image
		modifiedAt = subject.Article.ModifiedAt
		ogType, schemaType = "article", "Article"
		published := subject.Article.PublishedAt
		facts = subjectSchemaFacts{
			image: image, publishedAt: &published, modifiedAt: modifiedAt,
			authors: subject.Article.Authors, section: subject.Article.Section,
			tags: subject.Article.Tags,
		}
	case SubjectCollection:
		if subject.Collection == nil {
			err = failure(ErrorContract, "subject_kind_mismatch", "subject.kind", "does not match collection")
			return
		}
		title, description, image = subject.Collection.Title, subject.Collection.Description, subject.Collection.Image
		ogType, schemaType = "website", "CollectionPage"
		facts = subjectSchemaFacts{image: image}
	case SubjectProduct:
		if subject.Product == nil {
			err = failure(ErrorContract, "subject_kind_mismatch", "subject.kind", "does not match product")
			return
		}
		title, description, image = subject.Product.Name, subject.Product.Description, subject.Product.Image
		ogType, schemaType = "product", "Product"
		facts = subjectSchemaFacts{image: image, product: subject.Product}
	default:
		err = failure(ErrorContract, "unknown_subject_kind", "subject.kind", "is unknown")
		return
	}
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if title == "" {
		err = failure(ErrorContract, "subject_title_required", "subject", "title or name is required")
		return
	}
	return
}

func (module *Module) structuredData(
	page PageDescriptor,
	canonical string,
	locale string,
	schemaType string,
	facts subjectSchemaFacts,
) (map[string]any, error) {
	title, description, _, _, _, _, _, err := module.subjectFacts(page.Subject)
	if err != nil {
		return nil, err
	}
	graph := make([]map[string]any, 0, 3)
	websiteID := module.origin.String() + "/#website"
	graph = append(graph, compactMap(map[string]any{
		"@type":       "WebSite",
		"@id":         websiteID,
		"url":         module.origin.String() + "/",
		"name":        module.site.Name,
		"description": module.site.Description,
		"inLanguage":  module.site.DefaultLocale,
	}))
	pageNode := map[string]any{
		"@type":       schemaType,
		"@id":         canonical + "#primary",
		"url":         canonical,
		"name":        title,
		"headline":    title,
		"description": description,
		"inLanguage":  locale,
		"isPartOf":    map[string]any{"@id": websiteID},
	}
	if facts.image != nil {
		imageURL, err := module.absoluteURL(facts.image.URL, "subject.image.url")
		if err != nil {
			return nil, err
		}
		pageNode["image"] = compactMap(map[string]any{
			"@type":   "ImageObject",
			"url":     imageURL,
			"caption": facts.image.Alt,
			"width":   positiveOrNil(facts.image.Width),
			"height":  positiveOrNil(facts.image.Height),
		})
	}
	if facts.publishedAt != nil {
		pageNode["datePublished"] = facts.publishedAt.UTC().Format(time.RFC3339)
	}
	if facts.modifiedAt != nil {
		pageNode["dateModified"] = facts.modifiedAt.UTC().Format(time.RFC3339)
	}
	if len(facts.authors) > 0 {
		authors := make([]map[string]any, 0, len(facts.authors))
		for index, author := range facts.authors {
			if strings.TrimSpace(author.Name) == "" {
				return nil, failure(ErrorContract, "author_name_required", fmt.Sprintf("subject.article.authors.%d.name", index), "is required")
			}
			item := map[string]any{"@type": "Person", "name": author.Name}
			if author.URL != "" {
				authorURL, err := module.absoluteURL(author.URL, fmt.Sprintf("subject.article.authors.%d.url", index))
				if err != nil {
					return nil, err
				}
				item["url"] = authorURL
			}
			authors = append(authors, item)
		}
		pageNode["author"] = authors
	}
	if facts.section != "" {
		pageNode["articleSection"] = facts.section
	}
	if len(facts.tags) > 0 {
		pageNode["keywords"] = facts.tags
	}
	if facts.product != nil {
		pageNode["sku"] = emptyOrNil(facts.product.SKU)
		if facts.product.Brand != "" {
			pageNode["brand"] = map[string]any{"@type": "Brand", "name": facts.product.Brand}
		}
		if facts.product.Price != "" || facts.product.PriceCurrency != "" || facts.product.Availability != "" {
			if facts.product.Price == "" || facts.product.PriceCurrency == "" {
				return nil, failure(ErrorContract, "incomplete_product_offer", "subject.product", "price and priceCurrency must be supplied together")
			}
			pageNode["offers"] = compactMap(map[string]any{
				"@type": "Offer", "url": canonical,
				"price":         facts.product.Price,
				"priceCurrency": facts.product.PriceCurrency,
				"availability":  emptyOrNil(facts.product.Availability),
			})
		}
	}
	if len(page.Breadcrumbs) > 0 {
		items := make([]map[string]any, 0, len(page.Breadcrumbs))
		for index, breadcrumb := range page.Breadcrumbs {
			if strings.TrimSpace(breadcrumb.Name) == "" {
				return nil, failure(ErrorContract, "breadcrumb_name_required", fmt.Sprintf("breadcrumbs.%d.name", index), "is required")
			}
			itemURL, err := module.absoluteURL(breadcrumb.Path, fmt.Sprintf("breadcrumbs.%d.path", index))
			if err != nil {
				return nil, err
			}
			items = append(items, map[string]any{
				"@type": "ListItem", "position": index + 1,
				"name": breadcrumb.Name, "item": itemURL,
			})
		}
		breadcrumbID := canonical + "#breadcrumb"
		graph = append(graph, map[string]any{
			"@type": "BreadcrumbList", "@id": breadcrumbID, "itemListElement": items,
		})
		pageNode["breadcrumb"] = map[string]any{"@id": breadcrumbID}
	}
	graph = append(graph, compactMap(pageNode))
	result := map[string]any{
		"@context": "https://schema.org",
		"@graph":   graph,
	}
	nodes, tooDeep := structuredDataShape(result, 0)
	if tooDeep {
		return nil, failure(ErrorCapacity, "structured_data_depth_limit", "subject", "exceeds 32 levels")
	}
	if nodes > module.limits.MaxStructuredDataNodes {
		return nil, failure(ErrorCapacity, "structured_data_node_limit", "subject", "exceeds %d nodes", module.limits.MaxStructuredDataNodes)
	}
	return result, nil
}

func structuredDataShape(value any, depth int) (int, bool) {
	if depth > 32 {
		return 0, true
	}
	switch typed := value.(type) {
	case map[string]any:
		nodes := 1
		for _, item := range typed {
			count, tooDeep := structuredDataShape(item, depth+1)
			if tooDeep {
				return 0, true
			}
			nodes += count
		}
		return nodes, false
	case []map[string]any:
		nodes := 1
		for _, item := range typed {
			count, tooDeep := structuredDataShape(item, depth+1)
			if tooDeep {
				return 0, true
			}
			nodes += count
		}
		return nodes, false
	case []string:
		return 1 + len(typed), false
	default:
		return 1, false
	}
}

func compactMap(value map[string]any) map[string]any {
	for key, item := range value {
		switch typed := item.(type) {
		case nil:
			delete(value, key)
		case string:
			if typed == "" {
				delete(value, key)
			}
		}
	}
	return value
}

func positiveOrNil(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func emptyOrNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
