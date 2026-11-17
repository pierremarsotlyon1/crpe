import ReactMarkdown from "react-markdown";
import rehypeKatex from "rehype-katex";
import remarkMath from "remark-math";
import "katex/dist/katex.min.css";

export function Markdown({ children }: { children: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown
        skipHtml
        disallowedElements={["img"]}
        remarkPlugins={[remarkMath]}
        rehypePlugins={[
          [rehypeKatex, { trust: false, maxSize: 40, maxExpand: 500 }],
        ]}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}
