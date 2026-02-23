package main

import (
	"fmt"
	"os"
	"sort"

	vlt "github.com/RamXX/vlt"
)

func printVaults(vaults map[string]string, format string) {
	if len(vaults) == 0 {
		if format == "" {
			fmt.Println("No vaults found.")
		} else {
			formatList(nil, format)
		}
		return
	}

	names := make([]string, 0, len(vaults))
	for name := range vaults {
		names = append(names, name)
	}
	sort.Strings(names)
	formatVaults(names, vaults, format)
}

func dispatchRead(v *vlt.Vault, params map[string]string) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("read requires file=\"<title>\"")
	}
	content, err := v.Read(title, params["heading"])
	if err != nil {
		return err
	}
	fmt.Print(content)
	return nil
}

func dispatchSearch(v *vlt.Vault, params map[string]string, format string) error {
	query := params["query"]
	regexParam := params["regex"]
	contextStr := params["context"]
	pathFilter := params["path"]

	if query == "" && regexParam == "" {
		return fmt.Errorf("search requires query=\"<term>\" or regex=\"<pattern>\"")
	}

	// Parse context
	contextN := -1
	if contextStr != "" {
		n, err := vlt.ParseInt0(contextStr)
		if err != nil {
			return fmt.Errorf("invalid context value: %s", contextStr)
		}
		contextN = n
	}

	// Context mode
	if contextN >= 0 {
		matches, err := v.SearchWithContext(vlt.SearchOptions{
			Query:    query,
			Regex:    regexParam,
			Path:     pathFilter,
			ContextN: contextN,
		})
		if err != nil {
			return err
		}
		if len(matches) > 0 {
			formatSearchWithContext(matches, format)
		}
		return nil
	}

	// Non-context mode
	results, err := v.Search(vlt.SearchOptions{
		Query: query,
		Regex: regexParam,
		Path:  pathFilter,
	})
	if err != nil {
		return err
	}
	if len(results) > 0 {
		formatSearchResults(results, format)
	}
	return nil
}

func dispatchCreate(v *vlt.Vault, params map[string]string, silent bool, timestamps bool) error {
	name := params["name"]
	notePath := params["path"]

	if name == "" || notePath == "" {
		return fmt.Errorf("create requires name=\"<title>\" path=\"<relative-path>\"")
	}

	content := params["content"]
	if content == "" {
		content = readStdinIfPiped()
	}

	err := v.Create(name, notePath, content, silent, timestamps)
	if err != nil {
		if err == vlt.ErrNoteExists {
			if !silent {
				fmt.Fprintf(os.Stderr, "note already exists: %s\n", notePath)
			}
			return nil
		}
		return err
	}
	if !silent {
		fmt.Printf("created: %s\n", notePath)
	}
	return nil
}

func dispatchAppend(v *vlt.Vault, params map[string]string, timestamps bool) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("append requires file=\"<title>\"")
	}
	content := params["content"]
	if content == "" {
		content = readStdinIfPiped()
	}
	if content == "" {
		return fmt.Errorf("no content provided (use content=\"...\" or pipe to stdin)")
	}
	return v.Append(title, content, timestamps)
}

func dispatchPrepend(v *vlt.Vault, params map[string]string, timestamps bool) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("prepend requires file=\"<title>\"")
	}
	content := params["content"]
	if content == "" {
		content = readStdinIfPiped()
	}
	if content == "" {
		return fmt.Errorf("no content provided (use content=\"...\" or pipe to stdin)")
	}
	return v.Prepend(title, content, timestamps)
}

func dispatchWrite(v *vlt.Vault, params map[string]string, timestamps bool) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("write requires file=\"<title>\"")
	}
	content := params["content"]
	if content == "" {
		content = readStdinIfPiped()
	}
	return v.Write(title, content, timestamps)
}

func dispatchPatch(v *vlt.Vault, params map[string]string, delete bool, timestamps bool) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("patch requires file=\"<title>\"")
	}
	return v.Patch(title, vlt.PatchOptions{
		Heading:    params["heading"],
		LineSpec:   params["line"],
		Content:    params["content"],
		Delete:     delete,
		Timestamps: timestamps,
	})
}

func dispatchMove(v *vlt.Vault, params map[string]string) error {
	from := params["path"]
	to := params["to"]
	if from == "" || to == "" {
		return fmt.Errorf("move requires path=\"<from>\" to=\"<to>\"")
	}
	result, err := v.Move(from, to)
	if err != nil {
		return err
	}
	fmt.Printf("moved: %s -> %s\n", from, to)
	if result.WikilinksUpdated > 0 {
		oldTitle := result.OldTitle
		newTitle := result.NewTitle
		fmt.Printf("updated [[%s]] -> [[%s]] in %d file(s)\n", oldTitle, newTitle, result.WikilinksUpdated)
	}
	if result.MdLinksUpdated > 0 {
		fmt.Printf("updated [...](%s) -> [...](%s) in %d file(s)\n", from, to, result.MdLinksUpdated)
	}
	return nil
}

func dispatchDelete(v *vlt.Vault, params map[string]string, permanent bool) error {
	title := params["file"]
	notePath := params["path"]
	if title == "" && notePath == "" {
		return fmt.Errorf("delete requires file=\"<title>\" or path=\"<path>\"")
	}
	msg, err := v.Delete(title, notePath, permanent)
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}

func dispatchPropertySet(v *vlt.Vault, params map[string]string) error {
	title := params["file"]
	propName := params["name"]
	propValue := params["value"]
	if title == "" || propName == "" {
		return fmt.Errorf("property:set requires file=\"<title>\" name=\"<key>\" value=\"<val>\"")
	}
	if err := v.PropertySet(title, propName, propValue); err != nil {
		return err
	}
	fmt.Printf("set %s=%s in %q\n", propName, propValue, title)
	return nil
}

func dispatchPropertyRemove(v *vlt.Vault, params map[string]string) error {
	title := params["file"]
	propName := params["name"]
	if title == "" || propName == "" {
		return fmt.Errorf("property:remove requires file=\"<title>\" name=\"<key>\"")
	}
	if err := v.PropertyRemove(title, propName); err != nil {
		return err
	}
	fmt.Printf("removed %s from %q\n", propName, title)
	return nil
}

func dispatchProperties(v *vlt.Vault, params map[string]string, format string) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("properties requires file=\"<title>\"")
	}
	fm, err := v.Properties(title)
	if err != nil {
		return err
	}
	if fm != "" {
		formatProperties(fm, format)
	}
	return nil
}

func dispatchBacklinks(v *vlt.Vault, params map[string]string, format string) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("backlinks requires file=\"<title>\"")
	}
	results, err := v.Backlinks(title)
	if err != nil {
		return err
	}
	formatList(results, format)
	return nil
}

func dispatchLinks(v *vlt.Vault, params map[string]string, format string) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("links requires file=\"<title>\"")
	}
	links, err := v.Links(title)
	if err != nil {
		return err
	}
	if len(links) > 0 {
		formatLinks(links, format)
	}
	return nil
}

func dispatchOrphans(v *vlt.Vault, format string) error {
	orphans, err := v.Orphans()
	if err != nil {
		return err
	}
	formatList(orphans, format)
	return nil
}

func dispatchUnresolved(v *vlt.Vault, format string) error {
	results, err := v.Unresolved()
	if err != nil {
		return err
	}
	formatUnresolved(results, format)
	return nil
}

func dispatchTags(v *vlt.Vault, params map[string]string, showCounts bool, format string) error {
	tags, counts, err := v.Tags(params["sort"])
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	if showCounts || format != "" {
		formatTagCounts(tags, counts, format)
	} else {
		tagNames := make([]string, len(tags))
		for i, t := range tags {
			tagNames[i] = "#" + t
		}
		formatList(tagNames, format)
	}
	return nil
}

func dispatchTag(v *vlt.Vault, params map[string]string, format string) error {
	tag := params["tag"]
	if tag == "" {
		return fmt.Errorf("tag requires tag=\"<tagname>\"")
	}
	results, err := v.Tag(tag)
	if err != nil {
		return err
	}
	formatList(results, format)
	return nil
}

func dispatchFiles(v *vlt.Vault, params map[string]string, showTotal bool, format string) error {
	files, err := v.Files(params["folder"], params["ext"])
	if err != nil {
		return err
	}
	if showTotal {
		fmt.Println(len(files))
		return nil
	}
	formatList(files, format)
	return nil
}

func dispatchTasks(v *vlt.Vault, params map[string]string, flags map[string]bool) error {
	format := outputFormat(flags)
	tasks, err := v.Tasks(vlt.TaskOptions{
		File:    params["file"],
		Path:    params["path"],
		Done:    flags["done"],
		Pending: flags["pending"],
	})
	if err != nil {
		return err
	}
	outputTasks(tasks, format)
	return nil
}

func dispatchDaily(v *vlt.Vault, params map[string]string) error {
	result, err := v.Daily(params["date"])
	if err != nil {
		return err
	}
	if result.Created {
		fmt.Printf("created: %s\n", result.RelPath)
	} else {
		fmt.Print(result.Content)
	}
	return nil
}

func dispatchTemplates(v *vlt.Vault, params map[string]string, format string) error {
	templates, err := v.Templates()
	if err != nil {
		return err
	}
	formatList(templates, format)
	return nil
}

func dispatchTemplatesApply(v *vlt.Vault, params map[string]string) error {
	templateName := params["template"]
	noteName := params["name"]
	notePath := params["path"]

	if templateName == "" {
		return fmt.Errorf("templates:apply requires template=\"<name>\"")
	}
	if noteName == "" || notePath == "" {
		return fmt.Errorf("templates:apply requires name=\"<title>\" path=\"<path>\"")
	}

	if err := v.TemplatesApply(templateName, noteName, notePath); err != nil {
		return err
	}
	fmt.Printf("created: %s (from template %q)\n", notePath, templateName)
	return nil
}

func dispatchBookmarks(v *vlt.Vault, format string) error {
	paths, err := v.Bookmarks()
	if err != nil {
		return err
	}
	formatList(paths, format)
	return nil
}

func dispatchBookmarksAdd(v *vlt.Vault, params map[string]string) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("bookmarks:add requires file=\"<title>\"")
	}
	msg, err := v.BookmarksAdd(title)
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}

func dispatchBookmarksRemove(v *vlt.Vault, params map[string]string) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("bookmarks:remove requires file=\"<title>\"")
	}
	if err := v.BookmarksRemove(title); err != nil {
		return err
	}
	fmt.Printf("unbookmarked: %s\n", title)
	return nil
}

func dispatchURI(v *vlt.Vault, vaultName string, params map[string]string) error {
	title := params["file"]
	if title == "" {
		return fmt.Errorf("uri requires file=\"<title>\"")
	}
	uri, err := v.URI(vaultName, title, params["heading"], params["block"])
	if err != nil {
		return err
	}
	fmt.Println(uri)
	return nil
}
