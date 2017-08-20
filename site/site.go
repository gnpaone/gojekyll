package site

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/osteele/gojekyll/collection"
	"github.com/osteele/gojekyll/config"
	"github.com/osteele/gojekyll/pages"
	"github.com/osteele/gojekyll/plugins"
	"github.com/osteele/gojekyll/renderers"
	"github.com/osteele/gojekyll/utils"
	"github.com/osteele/liquid"
)

// Site is a Jekyll site.
type Site struct {
	Collections []*collection.Collection
	Routes      map[string]pages.Document // URL path -> Document, only for output pages

	config   config.Config
	data     map[string]interface{}
	flags    config.Flags
	themeDir string

	docs               []pages.Document // all documents, whether or not they are output
	nonCollectionPages []pages.Page

	renderer   *renderers.Manager
	renderOnce sync.Once

	drop     map[string]interface{} // cached drop value
	dropOnce sync.Once
}

// SourceDir returns the site source directory.
func (s *Site) SourceDir() string { return s.config.Source }

// DestDir returns the site destination directory.
func (s *Site) DestDir() string {
	if filepath.IsAbs(s.config.Destination) {
		return s.config.Destination
	}
	return filepath.Join(s.config.Source, s.config.Destination)
}

// OutputDocs returns a list of output pages.
func (s *Site) OutputDocs() []pages.Document {
	out := make([]pages.Document, 0, len(s.Routes))
	for _, p := range s.Routes {
		out = append(out, p)
	}
	return out
}

// Pages returns all the pages, output or not.
func (s *Site) Pages() (out []pages.Page) {
	for _, d := range s.docs {
		if p, ok := d.(pages.Page); ok {
			out = append(out, p)
		}
	}
	return
}

// AbsDir is in the collection.Site interface.
func (s *Site) AbsDir() string {
	d, err := filepath.Abs(s.SourceDir())
	if err != nil {
		panic(err)
	}
	return d
}

// Config is in the collection.Site interface.
func (s *Site) Config() *config.Config {
	return &s.config
}

func (s *Site) runHooks(h func(plugins.Plugin) error) error {
	for _, name := range s.config.Plugins {
		p, ok := plugins.Lookup(name)
		if ok {
			if err := h(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// Site is in the pages.RenderingContext interface.
func (s *Site) Site() interface{} {
	return s
}

// PathPrefix is in the page.Container interface.
func (s *Site) PathPrefix() string { return "" }

// New creates a new site record, initialized with the site defaults.
func New(flags config.Flags) *Site {
	s := &Site{config: config.Default(), flags: flags}
	s.config.ApplyFlags(flags)
	return s
}

// SetAbsoluteURL overrides the loaded configuration.
// The server uses this.
func (s *Site) SetAbsoluteURL(url string) {
	s.config.AbsoluteURL = url
	s.config.Variables["url"] = url
	if s.drop != nil {
		s.drop["url"] = url
	}
}

// FilenameURLs returns a map of site-relative pathnames to URL paths
func (s *Site) FilenameURLs() map[string]string {
	urls := map[string]string{}
	for _, page := range s.Pages() {
		urls[utils.MustRel(s.SourceDir(), page.SourcePath())] = page.Permalink()
	}
	return urls
}

// KeepFile returns a boolean indicating that clean should leave the file in the destination directory.
func (s *Site) KeepFile(filename string) bool {
	return utils.SearchStrings(s.config.KeepFiles, filename)
}

// FilePathPage returns a Page, give a file path relative to site source directory.
func (s *Site) FilePathPage(rel string) (pages.Document, bool) {
	// This looks wasteful. If it shows up as a hotspot, you know what to do.
	for _, p := range s.Routes {
		if p.SourcePath() != "" {
			if r, err := filepath.Rel(s.SourceDir(), p.SourcePath()); err == nil {
				if r == rel {
					return p, true
				}
			}
		}
	}
	return nil, false
}

// FilenameURLPath returns a page's URL path, give a relative file path relative to the site source directory.
func (s *Site) FilenameURLPath(relpath string) (string, bool) {
	if p, found := s.FilePathPage(relpath); found {
		return p.Permalink(), true
	}
	return "", false
}

// RendererManager returns the rendering manager.
func (s *Site) RendererManager() renderers.Renderers {
	if s.renderer == nil {
		panic(fmt.Errorf("uninitialized rendering manager"))
	}
	return s.renderer
}

// TemplateEngine is part of the plugins.Site interface.
func (s *Site) TemplateEngine() *liquid.Engine {
	return s.renderer.TemplateEngine()
}

// initializeRenderers initializes the rendering manager
func (s *Site) initializeRenderers() (err error) {
	options := renderers.Options{
		RelativeFilenameToURL: s.FilenameURLPath,
		ThemeDir:              s.themeDir,
	}
	s.renderer, err = renderers.New(s.config, options)
	if err != nil {
		return err
	}
	engine := s.renderer.TemplateEngine()
	return s.runHooks(func(p plugins.Plugin) error {
		return p.ConfigureTemplateEngine(engine)
	})
}

// RelativePath is in the page.Container interface.
func (s *Site) RelativePath(path string) string {
	if s.themeDir != "" {
		if rel, err := filepath.Rel(s.themeDir, path); err == nil {
			return rel
		}
	}
	return utils.MustRel(s.config.Source, path)
}

// URLPage returns the page that will be served at URL
func (s *Site) URLPage(urlpath string) (p pages.Document, found bool) {
	p, found = s.Routes[urlpath]
	if !found {
		p, found = s.Routes[filepath.Join(urlpath, "index.html")]
	}
	if !found {
		p, found = s.Routes[filepath.Join(urlpath, "index.htm")]
	}
	return
}

var excludeFileRE = regexp.MustCompile(`^[#~]|^\..|~$`)

// Exclude returns a boolean indicating that the site configuration excludes a file or directory.
// It does not exclude top-level _underscore files and directories.
func (s *Site) Exclude(siteRel string) bool {
	for siteRel != "." {
		dir, base := filepath.Dir(siteRel), filepath.Base(siteRel)
		switch {
		case utils.MatchList(s.config.Include, siteRel):
			return false
		case utils.MatchList(s.config.Exclude, siteRel):
			return true
		case dir != "." && base[0] == '_':
			return true
		default:
			if excludeFileRE.MatchString(base) {
				return true
			}
		}
		siteRel = dir
	}
	return false
}
