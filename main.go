package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/pkg/errors"
)

var dryRun = false

// Item represents the Qiita API's model:item
type Item struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	RenderedBody string    `json:"rendered_body"`
	Private      bool      `json:"private"`
	Tags         []Tagging `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Tagging represents the Qiita API's model:tagging
type Tagging struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

func main() {
	buf, err := ioutil.ReadFile("_posts/sample.md")
	if err != nil {
		panic(err)
	}
	it := ParseMarkdown(string(buf))
	r, err := PostNewItem(*it)
	if err != nil {
		panic(err)
	}
	st, err := r.ToMarkdown()
	if err != nil {
		panic(err)
	}
	println(st)
}

func (t Tagging) String() string {
	res := t.Name
	if len(t.Versions) > 0 {
		res = res + ":" + strings.Join(t.Versions, ",")
	}
	return res
}

// MarshalJSON is same as normal MarshalJSON except to emit null tags
func (t Tagging) MarshalJSON() ([]byte, error) {
	s := fmt.Sprintf(`{"name":"%s"`, t.Name)
	if t.Versions != nil {
		vs, err := json.Marshal(t.Versions)
		if err != nil {
			return nil, err
		}
		s = fmt.Sprintf(`%s, "tags":%s`, s, vs)
	}
	s = s + "}"
	return []byte(s), nil
}

// ToMarkdown converts Item to a markdown string
func (item *Item) ToMarkdown() (string, error) {
	var tagStrs []string
	for _, t := range item.Tags {
		tagStrs = append(tagStrs, t.String())
	}
	const templateText = `---
title: {{ .Title }}
tags:{{ range .Tags }} {{ . -}} {{ end }}
private: {{ .Private }}
---
{{ .Body }}`
	tpl, err := template.New("markdown").Parse(templateText)
	if err != nil {
		return "", err
	}
	var writer bytes.Buffer
	err = tpl.Execute(&writer, item)
	if err != nil {
		return "", err
	}
	return writer.String(), nil
}

// ParseMarkdown reads Qiita's Markdown text and generates Item from that
func ParseMarkdown(src string) *Item {
	var res Item
	res.Private = true
	sections := strings.SplitN(src, "---\n", 3)

	var meta map[string]interface{}
	yaml.Unmarshal([]byte(sections[1]), &meta)

	res.Body = sections[2]
	res.Title = meta["title"].(string)
	if priv, ok := meta["private"]; ok {
		res.Private = priv.(bool)
	}
	if tagv, ok := meta["tags"]; ok && tagv != nil {
		for _, t := range strings.Split(meta["tags"].(string), " ") {
			res.Tags = append(res.Tags, *ParseTagging(t))
		}
	}

	return &res
}

// ParseTagging generates Tagging from serialized string
func ParseTagging(src string) *Tagging {
	var res Tagging
	seps := strings.SplitN(src, ":", 2)
	res.Name = seps[0]
	if len(seps) == 2 {
		res.Versions = strings.Split(seps[1], ",")
	}
	return &res
}

func PostNewItem(item Item) (*Item, error) {
	postdata := map[string]interface{}{
		"body":    item.Body,
		"tags":    item.Tags,
		"title":   item.Title,
		"private": item.Private,
	}
	postbytes, err := json.Marshal(postdata)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest("POST", "https://qiita.com/api/v2/items", bytes.NewBuffer(postbytes))
	req.Header.Set("Content-Type", "application/json")

	resbytes, err := DoRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to DoRequest")
	}
	var res Item
	if err := json.Unmarshal(resbytes, &res); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal")
	}
	return &res, nil
}

// GetSelfItems fetch Items using Qiita API: authenticated_user/items
func GetSelfItems() ([]Item, error) {
	req, _ := http.NewRequest("GET", "https://qiita.com/api/v2/authenticated_user/items", nil)
	resbyte, err := DoRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to DoRequest")
	}

	var items []Item
	if err := json.Unmarshal(resbyte, &items); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal")
	}
	return items, nil
}

// DoRequest sends the request and return json response as interface{}
func DoRequest(req *http.Request) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+os.Getenv("QIITA_API_TOKEN"))
	if dryRun {
		a, _ := httputil.DumpRequest(req, true)
		fmt.Println(string(a))
		return []byte{}, nil
	}

	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to do request")
	}
	defer resp.Body.Close()

	// TODO: handling error status code

	res, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}
	return res, nil
}
