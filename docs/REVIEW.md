# Review notes

- Check Markdown source fidelity against unmodified source, independently of the exporter’s preprocessing. Reusing its heading/admonition preparation in the expected-value parser previously concealed corruption inside fenced examples.
- Keep Markdown discovery links conditional on export eligibility. Unlisted documents still have production HTML pages but intentionally have no Markdown export; drafts likewise have no production export.
- Preserve source fragment destinations independently of Markdown renderers’ automatic heading IDs. Explicit IDs do not consume Docusaurus heading counters, and synthetic titles must not steal section or non-heading anchors; collision checks must use semantic heading text before HTML serialization.
