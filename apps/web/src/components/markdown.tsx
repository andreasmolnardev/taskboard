import { useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeSanitize from 'rehype-sanitize';
import remarkGfm from 'remark-gfm';

export function MarkdownContent({ value, className = '' }: { value: string; className?: string }) {
  if (!value) return null;
  return (
    <div className={`markdown-content ${className}`}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
        {value}
      </ReactMarkdown>
    </div>
  );
}

const tools = [
  ['Heading', '# '], ['Bold', '**text**'], ['Italic', '*text*'], ['Strike', '~~text~~'],
  ['Link', '[text](https://example.com)'], ['Bulleted list', '- '], ['Numbered list', '1. '],
  ['Checklist', '- [ ] '], ['Code block', '```\ncode\n```'],
  ['Table', '| Column | Column |\n| --- | --- |\n| Value | Value |'],
] as const;

export function MarkdownEditor({
  value,
  onChange,
  rows = 4,
}: {
  value: string;
  onChange: (value: string) => void;
  rows?: number;
}) {
  const [preview, setPreview] = useState(false);
  const textarea = useRef<HTMLTextAreaElement>(null);
  const insert = (snippet: string) => {
    const input = textarea.current;
    if (!input) {
      onChange(`${value}${value ? '\n' : ''}${snippet}`);
      return;
    }
    const start = input.selectionStart;
    const end = input.selectionEnd;
    const next = `${value.slice(0, start)}${snippet}${value.slice(end)}`;
    onChange(next);
    requestAnimationFrame(() => {
      input.focus();
      input.setSelectionRange(start + snippet.length, start + snippet.length);
    });
  };

  return (
    <div className="markdown-editor">
      <div className="markdown-editor-toolbar" role="toolbar" aria-label="Markdown formatting">
        {tools.map(([label, snippet]) => (
          <button key={label} type="button" onClick={() => insert(snippet)}>{label}</button>
        ))}
        <button type="button" aria-pressed={preview} onClick={() => setPreview(!preview)}>
          {preview ? 'Edit' : 'Preview'}
        </button>
      </div>
      {preview ? (
        <MarkdownContent value={value} className="markdown-preview" />
      ) : (
        <textarea
          ref={textarea}
          className="composer-description"
          placeholder="Add a description (Markdown supported)"
          rows={rows}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </div>
  );
}
