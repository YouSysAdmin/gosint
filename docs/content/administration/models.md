---
title: Language models
description: "The models the assistant and the template generator talk to, and whose API key they use."
weight: 90
---

The [assistant](/guide/assistant/) and the transform template generator talk
to a language model. An install has **no model until an administrator adds
one**: until then, asking the assistant answers that no model is set up.

**Admin > Models > Add a model**. Several models can be offered, and each
person picks one in the chat. The first model added becomes the default.

## The fields

| Field | Default | Meaning |
| --- | --- | --- |
| **Name** | | What people see in the picker. Required |
| **API** | OpenAI style | **OpenAI style** speaks the chat completions API, which OpenAI, Mistral, OpenRouter, Ollama and LM Studio all serve. **Anthropic** speaks the Messages API |
| **Tools** | Auto | Whether the model is given tools (lookups, transform runs, proposals). **Auto** offers them to a remote service only, **On** always, **Off** never |
| **Base URL** | `http://localhost:1234/v1` | The API root: `https://api.openai.com/v1`, or the host alone for Anthropic |
| **Model** | | The model id. Empty on a local server answers with the model it has loaded |
| **Secret name** | `LLM_API_KEY` | Each person stores their own key in [Secrets](/guide/secrets/) under this name |
| **Server key** | | Optional. Used by anyone with no key of their own. Never shown again, **Remove the stored server key** clears it |
| **The default model, for anyone who picked none** | | Moves the default to this model |

A new model starts as LM Studio on this machine, which needs no key.

## Whose key is used

1. The person's own secret named by **Secret name**.
2. Otherwise the model's **Server key**.
3. A model on this machine or the local network (`localhost`, a loopback or a
   private address in **Base URL**) needs no key at all. The list marks such a
   model **local**.

Without any key for a remote model, the question is refused before anything is
sent, naming the secret to add.

## Tools, and why Auto

With tools the model can look things up, run transforms and propose changes
through structured calls. A model without tool support is told to write the
same requests as a fenced JSON block instead, and gosint reads them either
way.

**Auto** gives tools to a remote service and none to a local model, because
some local servers stall for minutes on a constrained tool call where the same
model answers the block form in seconds. If a remote model answers badly with
tools, set **Off**. If a capable local model handles tools well, set **On**.

## Privacy

What is selected in the sketch is sent to the model with the question: labels,
properties, relationships. Secret fields of an entity (passwords, hashes, API
keys found in an investigation) are named but never shown to the model. A
remote model is a third party: offer only the ones your investigations may be
sent to, or run one on your own hardware.

## Default and deletion

**Make default** in the list moves the default. Deleting the default hands it
to the first model left. People who picked a deleted model fall back to the
default.
