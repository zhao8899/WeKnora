import type {Tokens} from 'marked';
import {openMermaidFullscreen} from "@/utils/mermaidViewer.ts";
import {loadHighlightJs} from "@/utils/highlightRuntime";

let mermaidInitialized = false;
let mermaidModulePromise: Promise<any> | null = null;

const MERMAID_CONFIG = {
  startOnLoad: false,
  theme: 'default',
  securityLevel: 'strict',
  fontFamily: 'PingFang SC, Microsoft YaHei, sans-serif',
  flowchart: {
    useMaxWidth: true,
    htmlLabels: true,
    curve: 'basis',
  },
  sequence: {
    useMaxWidth: true,
    diagramMarginX: 8,
    diagramMarginY: 8,
    actorMargin: 50,
    width: 150,
    height: 65,
  },
  gantt: {
    useMaxWidth: true,
    leftPadding: 75,
    gridLineStartPadding: 35,
    barHeight: 20,
    barGap: 4,
    topPadding: 50,
  },
};

const loadMermaid = async () => {
  if (!mermaidModulePromise) {
    mermaidModulePromise = import('mermaid').then((mod) => mod.default);
  }
  const mermaid = await mermaidModulePromise;
  if (mermaidInitialized) return mermaid;
  mermaid.initialize(MERMAID_CONFIG as any);
  mermaidInitialized = true;
  return mermaid;
};

export const ensureMermaidInitialized = async () => {
  await loadMermaid();
};

let mermaidCount = 0;

export const createMermaidCodeRenderer = async (idPrefix: string) => {
  const hljs = await loadHighlightJs();
  return ({text, lang}: Tokens.Code) => {
    let highlighted = '';
    let highlightLang: string = lang || 'Code';
    if (highlightLang && hljs.getLanguage(highlightLang)) {
        try {
            highlighted = hljs.highlight(text, { language: highlightLang }).value;
        } catch {
            let ret = hljs.highlightAuto(text);
            highlighted = ret.value;
            highlightLang = ret.language || "Code";
        }
    } else {
        let ret = hljs.highlightAuto(text);
        highlighted = ret.value;
        highlightLang = ret.language || "Code";
    }
    if (lang === 'mermaid') {
      const id = `${idPrefix}-${++mermaidCount}`;
      return `<pre id="${id}" data-mermaid="false"><code class="hljs language-${highlightLang}">${highlighted}</code></pre>`;
    }
    return `<pre><code class="hljs language-${highlightLang}">${highlighted}</code></pre>`;
  };
};

export const renderMermaidInContainer = async (
  rootElement: HTMLElement | null | undefined,
) => {
  if (!rootElement) return 0;
  const mermaidElements = rootElement.querySelectorAll<HTMLElement>('pre[data-mermaid="false"]');
  if (mermaidElements.length === 0) return 0;

  const mermaid = await loadMermaid();
  for (const el of mermaidElements) {
    try{
        const code = el.innerText;
        // 验证
        await mermaid.parse(code);
        // 渲染
        let {svg} = await mermaid.render(`${el.id}-svg`, code);
        el.classList.add('mermaid')
        el.innerHTML = svg;
        el.onclick = (event) => {
            event.stopPropagation();
            openMermaidFullscreen(svg);
        };
    } catch(e){
        console.error("Mermaid rendering error:", e);
    }
    // 标记为已渲染
    el.setAttribute('data-mermaid', 'true');
  }
};
