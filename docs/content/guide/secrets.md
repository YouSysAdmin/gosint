---
title: Secrets
description: "Store the API keys and tokens your transforms, templates and the assistant use. They are yours alone and never shown again."
weight: 90
---

Many transforms call a service that wants an API key: Shodan, VirusTotal,
DeHashed, Hunter and others. You keep those keys in **Secrets**, in the top
bar.

## Storing a secret

Give it a **Name** and a **Value** and press **Store**. The name starts
with a letter or a digit and holds letters, digits, `_`, `.` and `-`, for
example `SHODAN_API_KEY`. Storing a name again replaces its value.

The value is encrypted at once and **never shown again**, not to you and
not to an administrator. To change it, store it again. To remove it,
press **Delete**.

## Yours alone

Secrets belong to your account. Your runs use your secrets, and nobody
else can use them or see their names. A colleague who runs the same
transform needs their own key.

Two cases spend your secrets without you pressing a button:

- a [scheduled flow](/guide/flows/#schedules) you created runs with your
  secrets every time
- a [template](/extending/transform-templates/) you wrote that reads
  secrets runs only for you, even when you share it, so that nobody can
  point it at a server of their own and collect your keys

## Which names to use

A transform asks for a secret by a fixed name, such as `ABUSEIPDB_API_KEY`.
The Secrets page helps:

- **Asked for, not stored yet** lists the names the built-in transforms and
  the assistant's models ask for that you have not stored. **Add** fills
  the name in for you.
- Each stored secret shows the transforms that use it, or "no built-in
  transform asks for it" for a name only your templates read.

When you run a transform, its secret parameter is filled with the name it
expects. You can type the name of another secret instead, for example to
use a second account's key. The run dialog warns when no secret by that
name is stored.

The names each transform reads are listed in
[Transforms](/reference/transforms/).

{{< callout type="note" title="What a scan records" >}}
A scan records the name of the secret it used, never the value. Logs and
the assistant see names only.
{{< /callout >}}
