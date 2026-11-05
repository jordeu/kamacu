import { expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { InboxText } from "./InboxText";

it("links plain URLs and shortens GitHub pull requests and issues", () => {
  const html = renderToStaticMarkup(<InboxText text="See https://example.com/docs, https://github.com/acme/widget/pull/123 and https://github.com/acme/widget/issues/456#issuecomment-1." />);
  expect(html).toContain('href="https://example.com/docs"');
  expect(html).toContain('href="https://github.com/acme/widget/pull/123"');
  expect(html).toContain('>widget#123</a>');
  expect(html).toContain('href="https://github.com/acme/widget/issues/456#issuecomment-1"');
  expect(html).toContain('>widget#456</a>');
  expect(html).toContain('target="_blank" rel="noopener noreferrer"');
});

it("supports Markdown links without shortening other GitHub URLs or lookalike hosts", () => {
  const html = renderToStaticMarkup(<InboxText text="[Docs](https://example.com) [PR](https://github.com/acme/widget/pull/42) https://github.com/acme/widget https://github.com.evil.test/acme/widget/issues/2" />);
  expect(html).toContain('>Docs</a>');
  expect(html).toContain('>widget#42</a>');
  expect(html).toContain('>https://github.com/acme/widget</a>');
  expect(html).toContain('>https://github.com.evil.test/acme/widget/issues/2</a>');
});

it("does not render executable URLs or raw HTML", () => {
  const html = renderToStaticMarkup(<InboxText text={'[unsafe](javascript:alert%281%29) <script>alert(1)</script>'} />);
  expect(html).not.toContain('href="javascript:');
  expect(html).not.toContain("<script>");
});
