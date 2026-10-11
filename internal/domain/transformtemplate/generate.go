// Gosint, Copyright (c) 2026 YouSysAdmin

package transformtemplate

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/llm"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/template"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/domain/chat"
)

// generateTimeout bounds one model call, under the server write timeout.
const generateTimeout = 100 * time.Second

// generatePrompt describes the template format to the model: lowercase
// type names, and output.relation for the edge.
const generatePrompt = `You write YAML transform templates for an OSINT graph tool. Given a description, answer with one valid template.

## Format

name: lowercase, hyphenated, unique (e.g. ip-api-lookup)
description: one sentence, what it finds
category: a word matching the input type (Ip, Domain, Email, Username, Organization...)
version: 1
input:
  type: the entity type it takes, lowercase (ip, domain, email, username, phone, organization, individual, cryptowallet, website, asn, cidr...)
  key: the input field placed into the request (ip: address, domain: domain, email: email, username: value, phone: number, organization: name, website: url, asn: number, cidr: network)
secrets: (optional) the API keys it needs, read from the person's secrets
  - name: SOME_API_KEY
    required: true
    description: what it is for
request:
  method: GET or POST
  url: with {{key}} placeholders, e.g. "https://api.example.com/lookup/{{address}}"
  headers: (optional) values may use {{secrets.NAME}}
  params: (optional) query parameters
  body: (optional) for POST
  timeout: seconds, default 30
response:
  expect: json, xml or text
  map: output field name -> path in the answer, dot notation for nesting (e.g. country: data.country)
output:
  type: the entity type it produces, lowercase
  is_array: true when the answer lists several results
  array_path: dot path to that list (e.g. data.results)
  relation: the edge label, UPPER_SNAKE_CASE (e.g. HAS_SOCIAL_ACCOUNT, RESOLVES_TO, RELATED_TO)

The same input and output type with no array means the answer is laid over the input entity's own properties.

## Rules

- Answer with the YAML only. No explanations, no markdown fences.
- Always quote a value holding {{...}}, unquoted braces are not YAML.
- response.map keys must be fields of the output type. The primary field of the output type must be mapped unless the output type equals the input type.
- Add a secrets section when the API needs a key, and use it as {{secrets.NAME}}.
- Keep it to what was asked.`

// generateInput is a description of the wanted transform.
type generateInput struct {
	Prompt     string `json:"prompt"      validate:"required,notblank,max=4000" normalize:"trim"`
	InputType  string `json:"input_type"  validate:"omitempty,nodetype"`
	OutputType string `json:"output_type" validate:"omitempty,nodetype"`
	ModelID    string `json:"model_id"    validate:"omitempty,uuid"`
}

// generateResponse is the YAML, with what the parser refuses in it when
// it refuses something, so the person can finish it by hand.
type generateResponse struct {
	Source  string `json:"source"`
	Problem string `json:"problem"`
}

// fenceRe strips a markdown code fence the model added anyway.
var fenceRe = regexp.MustCompile("(?s)^\\s*```[a-zA-Z]*\\n(.*?)\\n```\\s*$")

// placeholderRe finds an unquoted value holding a placeholder.
var placeholderRe = regexp.MustCompile(`(?m)^(\s*[\w-]+:\s+)([^"'\n]*\{\{[^"'\n]*)$`)

// Generate asks the model for a template matching the description.
func (h Handler) Generate(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[generateInput](c)
	if !ok {
		return resp
	}

	client, _, err := chat.ClientFor(c, h.Runtime, access.User(c).ID, in.ModelID)
	if chat.Refused(err) {
		return response.BadRequest(c, err.Error())
	}

	if err != nil {
		return response.Internal(c, err)
	}

	msgs := []llm.Message{{Role: llm.RoleSystem, Content: generatePrompt}}
	if ctx := typeContext(in.InputType, in.OutputType); ctx != "" {
		msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: ctx})
	}

	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: in.Prompt})

	// Fiber's request context never cancels, so the model call gets its
	// own bound, under the server's write timeout.
	ctx, cancel := context.WithTimeout(c, generateTimeout)
	defer cancel()

	answer, err := client.Complete(ctx, msgs)
	if err != nil {
		h.Runtime.Log.Warn("template generation failed", "err", err)

		return response.Coded(c, fiber.StatusBadGateway, "the model did not answer")
	}

	source := tidyYAML(answer)
	if strings.TrimSpace(source) == "" {
		return response.Coded(c, fiber.StatusBadGateway, "the model answered with no YAML, try a shorter description")
	}

	out := generateResponse{Source: source}
	if _, err := template.Parse([]byte(source), types.Builtin()); err != nil {
		out.Problem = err.Error()
	}

	return response.Success(c, out)
}

// typeContext lists the fields of the chosen types, so the model maps
// onto names that exist.
func typeContext(inputType, outputType string) string {
	reg := types.Builtin()
	var parts []string
	describe := func(role, name string) {
		t, ok := reg.Get(name)
		if !ok {
			return
		}

		var fields []string
		for _, f := range t.Fields {
			mark := ""
			if f.Primary {
				mark = " (primary)"
			}

			fields = append(fields, fmt.Sprintf("- %s: %s%s, %s", f.Name, f.Kind, mark, f.Description))
		}

		parts = append(parts, fmt.Sprintf("## %s type: %s\nThe template MUST use %s.type: %s. Its fields:\n%s",
			role, name, strings.ToLower(role), name, strings.Join(fields, "\n")))
	}

	describe("Input", inputType)
	describe("Output", outputType)

	return strings.Join(parts, "\n\n")
}

// tidyYAML strips a code fence and quotes bare placeholders, the two
// slips models make most.
func tidyYAML(answer string) string {
	s := strings.TrimSpace(answer)
	if m := fenceRe.FindStringSubmatch(s); m != nil {
		s = m[1]
	}

	s = placeholderRe.ReplaceAllString(s, `$1"$2"`)

	return strings.TrimSpace(s) + "\n"
}
