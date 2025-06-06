// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"strings"
	"time"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/markup"
)

const (
	tmplRepoWikiStart = "repo/wiki/start"
	tmplRepoWikiView  = "repo/wiki/view"
	tmplRepoWikiNew   = "repo/wiki/new"
	tmplRepoWikiPages = "repo/wiki/pages"
)

func MustEnableWiki(c *context.Context) {
	if !c.Repo.Repository.EnableWiki {
		c.NotFound()
		return
	}

	if c.Repo.Repository.EnableExternalWiki {
		c.Redirect(c.Repo.Repository.ExternalWikiURL)
		return
	}
}

type PageMeta struct {
	Name    string
	URL     string
	Updated time.Time
}

func renderWikiPage(c *context.Context, isViewPage bool) (*git.Repository, string) {
	wikiRepo, err := git.Open(c.Repo.Repository.WikiPath())
	if err != nil {
		c.Error(err, "open repository")
		return nil, ""
	}
	commit, err := wikiRepo.BranchCommit("master")
	if err != nil {
		c.Error(err, "get branch commit")
		return nil, ""
	}

	// Get page list.
	if isViewPage {
		entries, err := commit.Entries()
		if err != nil {
			c.Error(err, "list entries")
			return nil, ""
		}
		pages := make([]PageMeta, 0, len(entries))
		for i := range entries {
			if entries[i].Type() == git.ObjectBlob && strings.HasSuffix(entries[i].Name(), ".md") {
				name := strings.TrimSuffix(entries[i].Name(), ".md")
				pages = append(pages, PageMeta{
					Name: name,
					URL:  database.ToWikiPageURL(name),
				})
			}
		}
		c.Data["Pages"] = pages
	}

	pageURL := c.Params(":page")
	if pageURL == "" {
		pageURL = "Home"
	}
	c.Data["PageURL"] = pageURL

	pageName := database.ToWikiPageName(pageURL)
	c.Data["old_title"] = pageName
	c.Data["Title"] = pageName
	c.Data["title"] = pageName
	c.Data["RequireHighlightJS"] = true

	blob, err := commit.Blob(pageName + ".md")
	if err != nil {
		if gitutil.IsErrRevisionNotExist(err) {
			c.Redirect(c.Repo.RepoLink + "/wiki/_pages")
		} else {
			c.Error(err, "get blob")
		}
		return nil, ""
	}
	p, err := blob.Bytes()
	if err != nil {
		c.Error(err, "read blob")
		return nil, ""
	}
	if isViewPage {
		c.Data["content"] = string(markup.Markdown(p, c.Repo.RepoLink, c.Repo.Repository.ComposeMetas()))
	} else {
		c.Data["content"] = string(p)
	}

	return wikiRepo, pageName
}

func Wiki(c *context.Context) {
	c.Data["PageIsWiki"] = true

	if !c.Repo.Repository.HasWiki() {
		c.Data["Title"] = c.Tr("repo.wiki")
		c.Success(tmplRepoWikiStart)
		return
	}

	wikiRepo, pageName := renderWikiPage(c, true)
	if c.Written() {
		return
	}

	// Get last change information.
	commits, err := wikiRepo.Log(git.RefsHeads+"master", git.LogOptions{Path: pageName + ".md"})
	if err != nil {
		c.Error(err, "get commits by path")
		return
	}
	c.Data["Author"] = commits[0].Author

	c.Success(tmplRepoWikiView)
}

func WikiPages(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.wiki.pages")
	c.Data["PageIsWiki"] = true

	if !c.Repo.Repository.HasWiki() {
		c.Redirect(c.Repo.RepoLink + "/wiki")
		return
	}

	wikiRepo, err := git.Open(c.Repo.Repository.WikiPath())
	if err != nil {
		c.Error(err, "open repository")
		return
	}
	commit, err := wikiRepo.BranchCommit("master")
	if err != nil {
		c.Error(err, "get branch commit")
		return
	}

	entries, err := commit.Entries()
	if err != nil {
		c.Error(err, "list entries")
		return
	}
	pages := make([]PageMeta, 0, len(entries))
	for i := range entries {
		if entries[i].Type() == git.ObjectBlob && strings.HasSuffix(entries[i].Name(), ".md") {
			commits, err := wikiRepo.Log(git.RefsHeads+"master", git.LogOptions{Path: entries[i].Name()})
			if err != nil {
				c.Error(err, "get commits by path")
				return
			}
			name := strings.TrimSuffix(entries[i].Name(), ".md")
			pages = append(pages, PageMeta{
				Name:    name,
				URL:     database.ToWikiPageURL(name),
				Updated: commits[0].Author.When,
			})
		}
	}
	c.Data["Pages"] = pages

	c.Success(tmplRepoWikiPages)
}

func NewWiki(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.wiki.new_page")
	c.Data["PageIsWiki"] = true
	c.Data["RequireSimpleMDE"] = true

	if !c.Repo.Repository.HasWiki() {
		c.Data["title"] = "Home"
	}

	c.Success(tmplRepoWikiNew)
}

func NewWikiPost(c *context.Context, f form.NewWiki) {
	c.Data["Title"] = c.Tr("repo.wiki.new_page")
	c.Data["PageIsWiki"] = true
	c.Data["RequireSimpleMDE"] = true

	if c.HasError() {
		c.Success(tmplRepoWikiNew)
		return
	}

	if err := c.Repo.Repository.AddWikiPage(c.User, f.Title, f.Content, f.Message); err != nil {
		if database.IsErrWikiAlreadyExist(err) {
			c.Data["Err_Title"] = true
			c.RenderWithErr(c.Tr("repo.wiki.page_already_exists"), tmplRepoWikiNew, &f)
		} else {
			c.Error(err, "add wiki page")
		}
		return
	}

	c.Redirect(c.Repo.RepoLink + "/wiki/" + database.ToWikiPageURL(database.ToWikiPageName(f.Title)))
}

func EditWiki(c *context.Context) {
	c.Data["PageIsWiki"] = true
	c.Data["PageIsWikiEdit"] = true
	c.Data["RequireSimpleMDE"] = true

	if !c.Repo.Repository.HasWiki() {
		c.Redirect(c.Repo.RepoLink + "/wiki")
		return
	}

	renderWikiPage(c, false)
	if c.Written() {
		return
	}

	c.Success(tmplRepoWikiNew)
}

func EditWikiPost(c *context.Context, f form.NewWiki) {
	c.Data["Title"] = c.Tr("repo.wiki.new_page")
	c.Data["PageIsWiki"] = true
	c.Data["RequireSimpleMDE"] = true

	if c.HasError() {
		c.Success(tmplRepoWikiNew)
		return
	}

	if err := c.Repo.Repository.EditWikiPage(c.User, f.OldTitle, f.Title, f.Content, f.Message); err != nil {
		c.Error(err, "edit wiki page")
		return
	}

	c.Redirect(c.Repo.RepoLink + "/wiki/" + database.ToWikiPageURL(database.ToWikiPageName(f.Title)))
}

func DeleteWikiPagePost(c *context.Context) {
	pageURL := c.Params(":page")
	if pageURL == "" {
		pageURL = "Home"
	}

	pageName := database.ToWikiPageName(pageURL)
	if err := c.Repo.Repository.DeleteWikiPage(c.User, pageName); err != nil {
		c.Error(err, "delete wiki page")
		return
	}

	c.JSONSuccess(map[string]any{
		"redirect": c.Repo.RepoLink + "/wiki/",
	})
}
