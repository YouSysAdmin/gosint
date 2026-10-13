# gosint documentation

A Hugo site published on GitHub Pages by `.github/workflows/docs.yaml`
on every push to master that touches `docs/`. The theme is
[hugo-docs-theme](https://github.com/YouSysAdmin/hugo-docs-theme), a
Hugo module pinned in `go.mod`, so a build needs Go beside Hugo 0.158 or
newer.

```bash
task docs-dev   # live editing on http://localhost:1313/gosint/
task docs       # build into docs/public
task docs-gen   # rewrite the generated reference pages
```

## Layout

```text
content/
  getting-started/   install, first run, a first investigation, the desktop app
  guide/             using the console
  administration/    running a server
  extending/         templates, transform files, custom types
  reference/         configuration keys, the command line, the keyboard,
                     and two GENERATED pages: entity-types.md, transforms.md
assets/images/       the screenshot the landing page and the README show
```

`reference/entity-types.md` and `reference/transforms.md` are written by
`internal/tools/docgen` from the type registry and the transform list.
Edit the code and run `task docs-gen`, never the pages.

## Writing a page

Front matter: `title`, `description` (quoted, it may hold a colon) and
`weight`, which orders the sidebar. A section is a directory with an
`_index.md` that also names an `icon` from the theme's `assets/icons`.
Links between pages are root-relative (`/guide/flows/`), the theme
resolves them against `baseURL`.

Callouts:

```
{{< callout type="warning" title="Optional title" >}}
Body in **markdown**.
{{< /callout >}}
```

Types: `note`, `info`, `tip`, `success`, `warning`, `danger`.

To try a change to the theme before tagging it:

```bash
HUGO_MODULE_REPLACEMENTS="github.com/YouSysAdmin/hugo-docs-theme -> /path/to/hugo-docs-theme" hugo server
```
