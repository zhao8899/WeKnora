let highlightRuntimePromise: Promise<any> | null = null;

export const loadHighlightJs = async () => {
  if (!highlightRuntimePromise) {
    highlightRuntimePromise = Promise.all([
      import("highlight.js/styles/github.css"),
      import("highlight.js/lib/core"),
      import("highlight.js/lib/languages/javascript"),
      import("highlight.js/lib/languages/typescript"),
      import("highlight.js/lib/languages/python"),
      import("highlight.js/lib/languages/bash"),
      import("highlight.js/lib/languages/json"),
      import("highlight.js/lib/languages/xml"),
      import("highlight.js/lib/languages/css"),
      import("highlight.js/lib/languages/yaml"),
      import("highlight.js/lib/languages/go"),
      import("highlight.js/lib/languages/java"),
      import("highlight.js/lib/languages/cpp"),
      import("highlight.js/lib/languages/sql"),
      import("highlight.js/lib/languages/rust"),
      import("highlight.js/lib/languages/ruby"),
      import("highlight.js/lib/languages/php"),
      import("highlight.js/lib/languages/markdown"),
    ]).then(
      ([
        _style,
        core,
        javascript,
        typescript,
        python,
        bash,
        json,
        xml,
        css,
        yaml,
        go,
        java,
        cpp,
        sql,
        rust,
        ruby,
        php,
        markdown,
      ]) => {
        const hljs = core.default;

        [
          ["javascript", javascript.default],
          ["typescript", typescript.default],
          ["python", python.default],
          ["bash", bash.default],
          ["json", json.default],
          ["xml", xml.default],
          ["css", css.default],
          ["yaml", yaml.default],
          ["go", go.default],
          ["java", java.default],
          ["cpp", cpp.default],
          ["sql", sql.default],
          ["rust", rust.default],
          ["ruby", ruby.default],
          ["php", php.default],
          ["markdown", markdown.default],
        ].forEach(([name, language]) => {
          if (!hljs.getLanguage(name as string)) {
            hljs.registerLanguage(name as string, language as any);
          }
        });

        hljs.registerAliases(["js", "jsx"], { languageName: "javascript" });
        hljs.registerAliases(["ts", "tsx"], { languageName: "typescript" });
        hljs.registerAliases(["sh", "shell"], { languageName: "bash" });
        hljs.registerAliases(["yml"], { languageName: "yaml" });
        hljs.registerAliases(["html"], { languageName: "xml" });
        hljs.registerAliases(["c", "h", "cc", "cxx", "hpp"], { languageName: "cpp" });
        hljs.registerAliases(["md"], { languageName: "markdown" });
        hljs.registerAliases(["mermaid"], { languageName: "markdown" });

        return hljs;
      },
    );
  }

  return highlightRuntimePromise;
};
