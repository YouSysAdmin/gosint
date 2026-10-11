// Gosint, Copyright (c) 2026 YouSysAdmin

// Package transformtemplate stores YAML transforms, serves their editing
// and test endpoints, and launches one on nodes of a sketch.
//
// A PUBLIC TEMPLATE THAT READS SECRETS RUNS FOR ITS OWNER ONLY. The
// secrets a template asks for are opened from the secrets of whoever runs
// it, and the template's author picks where the request goes, so a
// shared template could otherwise collect other people's API keys.
package transformtemplate
