---
title: Assistant
description: "Ask a language model about the selected entities. It looks things up, runs transforms when you could, and proposes changes for you to apply."
weight: 80
---

The assistant is a chat beside the graph, answered by a language model an
administrator set up (see [Models](/administration/models/)). You select
entities, ask a question, and the model answers about them. It can read
web pages and the graph, run transforms and flows, and propose changes. It
never edits the graph by itself: every change is a card you apply.

## Opening a chat

Press **Chat** in the sketch header. The panel opens beside the graph.

- The select at the top lists your chats on this sketch. The chats about
  the entities you have selected come first, under **About the selected
  entities**, the rest under **Other chats**. Selecting other entities opens
  their newest chat.
- **New** starts another chat, the cross deletes the current one.
- The first question names the chat.

Chats are yours. Other members do not see them.

## Asking

Type the question and press Enter (Shift+Enter for a new line). With
entities selected, two boxes decide what the model sees:

- **Use the N selected entities as context** hands them over, one per line,
  with their fields and their id.
- **Also what was found through them** adds the entities that hang from
  them.

Fields marked secret (a password, a key, a hash) are named to the model but
never shown to it. The model may be a remote service, so think about what
you select.

**Stop** ends an answer in progress. When the model reasons before it
answers, the panel counts the characters it thinks and **Show the
reasoning** opens them.

## Choosing a model

When the server offers more than one model, a select beside **Send** picks
the one that answers. Your pick is remembered in this browser. The panel
says which model answers and, for a remote one, which secret it uses.

## The API key

A model run by a remote service needs an API key. Each person uses their
own: the key is a [secret](/guide/secrets/) named as the model says,
`LLM_API_KEY` unless the administrator chose another name. If you have not
stored one, the question is refused before anything is sent, with a link to
Secrets. When the administrator gave the model a key of the server's, that
one is used when you have none. A model running on the same machine or
network as the server needs no key.

## What it looks up

While answering, the model can:

- **fetch a web page** and read its text (through the same guarded client
  as the transforms, so never an internal address)
- read **an entity and its relationships** in this sketch
- **find an entity** by its label

Each lookup is listed under the answer as it happens.

## Running transforms

When your role lets you edit the sketch, the model can run a transform or a
flow on the selected entities, or on entities it names. It sees which
transforms take the selected types and what they give, so it can chain
them on the spot: "find the subdomains, their addresses, drop the CDN ones,
and list the open ports".

The run is an ordinary scan in the **Scans** tab. The model waits for it
(up to four minutes), reads what it found and goes on answering. A longer
run keeps going and the model says so.

Some runs are not started for you. They come back as a card with a **Run**
button, and start only when you press it:

- a run that contacts the target, such as `ip_to_ports` with the active
  source
- a run that spends a secret, such as a paid API
- a transform that sends data to an address it was given

## Proposals

When the model has something to add, it proposes changes as cards under
its answer:

- set fields on an entity, showing the old value and the new one
- add an entity, linked to one already in the sketch
- link two entities

Press **Apply** on a card, or **Apply all**. Each change goes through the
entity types like a hand edit, so a value the type refuses is reported and
skipped. The entities record that they were "added from the assistant".
Applied proposals are not part of undo.

## Viewers

A viewer can chat too, but the chat is read only: the model is told so,
offers no runs, and the cards cannot be applied.
