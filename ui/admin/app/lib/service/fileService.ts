export const MimeType = {
    Pdf: { mimeType: "application/pdf", extension: "pdf" },
    Html: { mimeType: "text/html", extension: "html" },
    Markdown: { mimeType: "text/markdown", extension: "md" },
    Text: { mimeType: "text/plain", extension: "txt" },
    OpenDocument: {
        mimeType: "application/vnd.oasis.opendocument.text",
        extension: "odt",
    },
    DocX: {
        mimeType:
            "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        extension: "docx",
    },
    RichText: { mimeType: "application/rtf", extension: "rtf" },
    Csv: { mimeType: "text/csv", extension: "csv" },
    Jupyter: { mimeType: "application/x-ipynb+json", extension: "ipynb" },
    Json: { mimeType: "application/json", extension: "json" },
} as const;

export const KnowledgeAcceptedMimeTypes = [
    MimeType.Pdf,
    MimeType.Html,
    MimeType.Markdown,
    MimeType.Text,
    MimeType.OpenDocument,
    MimeType.DocX,
    MimeType.RichText,
    MimeType.Csv,
    MimeType.Jupyter,
    MimeType.Json,
]
    .map((type) => type.mimeType)
    .join(",");
