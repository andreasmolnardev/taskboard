import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { MarkdownContent } from './markdown';

describe('MarkdownContent', () => {
  it('renders common Markdown and GitHub-flavored tables', () => {
    const html = renderToStaticMarkup(
      <MarkdownContent value={'## Goal\n\n- [ ] Ship it\n\n| A | B |\n| - | - |\n| 1 | 2 |'} />,
    );
    expect(html).toContain('<h2>Goal</h2>');
    expect(html).toContain('<table>');
    expect(html).toContain('Ship it');
  });

  it('does not render raw HTML or unsafe links', () => {
    const html = renderToStaticMarkup(
      <MarkdownContent value={'<img src=x onerror="alert(1)">\n\n[bad](javascript:alert(1))'} />,
    );
    expect(html).not.toContain('<img');
    expect(html).not.toContain('onerror');
    expect(html).not.toContain('javascript:');
  });
});
