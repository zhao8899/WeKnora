// @ts-nocheck
<script setup lang="ts">
import { defineAsyncComponent, onMounted, ref, nextTick, onUnmounted, watch } from "vue";
import { downKnowledgeDetails, deleteGeneratedQuestion, getChunkByIdOnly } from "@/api/knowledge-base/index";
import { DialogPlugin } from "tdesign-vue-next/es/dialog";
import { MessagePlugin } from "tdesign-vue-next/es/message";
import { sanitizeHTML, safeMarkdownToHTML, createSafeImage, isValidImageURL, hydrateProtectedFileImages } from '@/utils/security';
import { loadHighlightJs } from '@/utils/highlightRuntime';
import { renderMermaidInContainer } from '@/utils/mermaidShared';
import { useI18n } from 'vue-i18n';
const DocumentPreview = defineAsyncComponent(() => import('@/components/document-preview.vue'));

const { t } = useI18n();
let markedModulePromise: Promise<any> | null = null;
let renderer: any = null;
let markdownRenderSeq = 0;

// Mermaid 閸掓繂顫愰崠鏍吀閺佹澘娅掗敍宀€鏁ゆ禍搴ｆ晸閹存劕鏁稉鈧琁D
let mermaidRenderCount = 0;
const props = defineProps(["visible", "details", "knowledgeType", "sourceInfo"]);
const emit = defineEmits(["closeDoc", "getDoc", "questionDeleted"]);
let page = 1;
let loadingChunks = false;
let pendingRequestedPage: number | null = null;
let pendingChunksBeforeLoad = 0;
let doc: HTMLElement | null = null;
let down = ref()
let mdContentWrap = ref<HTMLElement | null>(null)
let url = ref('')
const viewMode = ref<'chunks' | 'merged' | 'preview'>('merged');

const mergedContent = ref<string>('');
const renderedMergedContent = ref('');
const renderedChunkContent = ref<Map<string, string>>(new Map());
const renderedParentContent = ref<Map<string, string>>(new Map());

const loadMarkdownRuntime = async () => {
  if (!markedModulePromise) {
    markedModulePromise = import('marked').then(({ marked }) => marked);
  }
  const [marked, hljs] = await Promise.all([markedModulePromise, loadHighlightJs()]);

  if (!renderer) {
    marked.use({ breaks: true, gfm: true });
    renderer = new marked.Renderer();
    renderer.image = function ({href, title, text}: { href: string; title?: string; text?: string }) {
      if (!isValidImageURL(href)) {
        return `<p>${t('error.invalidImageLink')}</p>`;
      }
      const safeImage = createSafeImage(href, text || '', title || '');
      return `<figure>${safeImage}<figcaption style="text-align: left;">${text || ''}</figcaption></figure>`;
    };
    renderer.code = function ({text, lang}: { text: string; lang?: string }) {
      let detectedLang = lang;
      let highlighted = '';
      if (lang && hljs.getLanguage(lang)) {
        try {
          highlighted = hljs.highlight(text, { language: lang }).value;
        } catch (e) {
          const autoFallback = hljs.highlightAuto(text);
          highlighted = autoFallback.value;
          detectedLang = autoFallback.language || lang;
        }
      } else {
        const auto = hljs.highlightAuto(text);
        highlighted = auto.value;
        detectedLang = auto.language || lang;
      }
      if (lang === 'mermaid') {
        const id = `doc-mermaid-${++mermaidRenderCount}`;
        return `<pre id="${id}" data-mermaid="false"><code class="hljs language-${detectedLang || 'mermaid'}">${highlighted}</code></pre>`;
      }
      const displayLang = detectedLang || 'Code';
      return `<div class="code-block-wrapper"><div class="code-block-header"><span class="code-block-lang">${displayLang}</span></div><pre class="code-block-pre"><code class="hljs language-${detectedLang || ''}">${highlighted}</code></pre></div>`;
    };
  }

  return { marked };
};

const renderMarkdownToHTML = async (markdownText: string): Promise<string> => {
  if (!markdownText || typeof markdownText !== 'string') return '';
  let processedText = markdownText
    .replace(/&#39;/g, "'")
    .replace(/&#x27;/gi, "'")
    .replace(/&apos;/g, "'")
    .replace(/&#34;/g, '"')
    .replace(/&#x22;/gi, '"')
    .replace(/&quot;/g, '"')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');
  processedText = processedText.replace(/<p>\s*(\|[\s\S]*?\|)\s*<\/p>/gi, '\n$1\n');
  const safeMarkdown = safeMarkdownToHTML(processedText);
  const { marked } = await loadMarkdownRuntime();
  marked.use({ renderer });
  let html = String(marked.parse(safeMarkdown, { async: false }));
  html = html.replace(/&lt;br\s*\/?&gt;/gi, '<br>');
  return sanitizeHTML(html);
};

const refreshRenderedMarkdown = async () => {
  if (!props.visible || (viewMode.value !== 'chunks' && viewMode.value !== 'merged')) return;
  const seq = ++markdownRenderSeq;
  const nextMergedContent = mergedContent.value ? await renderMarkdownToHTML(mergedContent.value) : '';
  if (seq !== markdownRenderSeq) return;
  const nextChunkContent = new Map<string, string>();
  for (const item of (props.details?.md || [])) {
    const chunkContent = item?.content || '';
    if (!chunkContent) continue;
    nextChunkContent.set(chunkContent, await renderMarkdownToHTML(chunkContent));
    if (seq !== markdownRenderSeq) return;
  }
  const nextParentContent = new Map<string, string>();
  for (const [parentId, parentContent] of parentContextCache.value.entries()) {
    if (!parentContent) continue;
    nextParentContent.set(parentId, await renderMarkdownToHTML(parentContent));
    if (seq !== markdownRenderSeq) return;
  }
  renderedMergedContent.value = nextMergedContent;
  renderedChunkContent.value = nextChunkContent;
  renderedParentContent.value = nextParentContent;
  await runMarkdownPostRenderPipeline();
};

const getRenderedChunkHTML = (content: string) => renderedChunkContent.value.get(content || '') || '';
const getRenderedParentHTML = (item: any) => renderedParentContent.value.get(item?.parent_chunk_id) || '';

/**
 * 閺嶈宓?start_at 閸?end_at 鐎涙顔岄崥鍫濊嫙閺?overlap 閻?chunks
 * 鏉╂柨娲栭崥鍫濊嫙閸氬海娈戠€瑰本鏆ｉ弬鍥ㄣ€傞崘鍛啇
 * 鐎圭偟骞囬柅鏄忕帆娑撳骸鎮楃粩?Go 娴狅絿鐖滄穱婵囧瘮娑撯偓閼?
 */
const mergeChunks = (chunks: any[]): string => {
  if (!chunks || chunks.length === 0) return '';
  
  // 閹?start_at 閹烘帒绨?
  const sortedChunks = [...chunks].sort((a, b) => {
    const startA = a.start_at ?? a.chunk_index ?? 0;
    const startB = b.start_at ?? b.chunk_index ?? 0;
    return startA - startB;
  });
  
  // 閸掓繂顫愰崠鏍ф値楠炲墎绮ㄩ弸婊愮礉缁楊兛绔存稉?chunk 閻╁瓨甯撮崝鐘插弳
  const mergedChunks: Array<{
    content: string;
    start_at: number;
    end_at: number;
  }> = [{
    content: sortedChunks[0].content || '',
    start_at: sortedChunks[0].start_at ?? 0,
    end_at: sortedChunks[0].end_at ?? 0
  }];
  
  // 娴犲海顑囨禍灞奸嚋 chunk 瀵偓婵浜堕崢?
  for (let i = 1; i < sortedChunks.length; i++) {
    const currentChunk = sortedChunks[i];
    const lastChunk = mergedChunks[mergedChunks.length - 1];
    
    const currentStartAt = currentChunk.start_at ?? 0;
    const currentEndAt = currentChunk.end_at ?? 0;
    const currentContent = currentChunk.content || '';
    
    // 婵″倹鐏夎ぐ鎾冲 chunk 閻ㄥ嫯鎹ｆ慨瀣╃秴缂冾喖婀張鈧崥搴濈娑?chunk 閻ㄥ嫮绮ㄩ弶鐔剁秴缂冾喕绠ｉ崥搴礉閻╁瓨甯村ǎ璇插
    if (currentStartAt > lastChunk.end_at) {
      mergedChunks.push({
        content: currentContent,
        start_at: currentStartAt,
        end_at: currentEndAt
      });
      continue;
    }
    
    // 閸氬牆鑻熼柌宥呭綌閻?chunks
    if (currentEndAt > lastChunk.end_at) {
      // 鐏忓棗鍞寸€圭娴嗛幑顫礋鐎涙顑侀弫鎵矋娴犮儲顒滅涵顔碱槱閻炲棗顦跨€涙濡€涙顑?
      const contentRunes = Array.from(currentContent);
      const contentLength = contentRunes.length;
      
      // 鐠侊紕鐣婚崑蹇曅╅柌蹇ョ窗閸愬懎顔愰梹鍨 - (瑜版挸澧犵紒鎾存将娴ｅ秶鐤?- 娑撳﹣绔存稉顏嗙波閺夌喍缍呯純?
      const offset = contentLength - (currentEndAt - lastChunk.end_at);
      
      // 閹峰吋甯撮棃鐐哄櫢閸欑娀鍎撮崚?
      const newContent = contentRunes.slice(offset).join('');
      lastChunk.content = lastChunk.content + newContent;
      lastChunk.end_at = currentEndAt;
    }
  }
  
  // 閸氬牆鑻熼幍鈧張澶嬵唽閽€鏂ょ礉閻劌寮婚幑銏ｎ攽缁楋箒绻涢幒?
  return mergedChunks.map(chunk => chunk.content).join('\n\n');
};

onMounted(() => {
  nextTick(() => {
    doc = document.getElementsByClassName('t-drawer__body')[0] as HTMLElement | undefined || null
    doc?.addEventListener('scroll', handleDetailsScroll);
  })
})
watch(() => props.details?.id, () => {
  page = 1;
  loadingChunks = false;
  pendingRequestedPage = null;
  pendingChunksBeforeLoad = 0;
});
watch(() => props.details?.chunkLoading, (val) => {
  if (val === false) {
    if (pendingRequestedPage !== null) {
      const currentLength = props.details?.md?.length || 0;
      const hasError = Boolean(props.details?.chunkLoadError);
      if (hasError && currentLength <= pendingChunksBeforeLoad) {
        page = Math.max(1, pendingRequestedPage - 1);
        MessagePlugin.warning(props.details?.chunkLoadError);
      }
    }
    pendingRequestedPage = null;
    pendingChunksBeforeLoad = 0;
    loadingChunks = false;
  }
});
onUnmounted(() => {
  doc?.removeEventListener('scroll', handleDetailsScroll);
})
const checkImage = (url: string): Promise<boolean> => {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => resolve(true);
    img.onerror = () => resolve(false);
    img.src = url;
  });
};
// 閻╂垵鎯?chunks 閸欐ê瀵查敍宀冨殰閸斻劍娲块弬鏉挎値楠炶泛鍞寸€?
watch(() => props.details?.md, (newChunks) => {
  if (newChunks && newChunks.length > 0) {
    mergedContent.value = mergeChunks(newChunks);
  } else {
    mergedContent.value = '';
  }
}, { immediate: true, deep: true });

const previewSupportedTypes = new Set([
  'pdf', 'docx', 'pptx', 'ppt', 'xlsx', 'xls', 'csv',
  'jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'tiff', 'svg',
  'txt', 'md', 'markdown', 'json', 'xml', 'html', 'css', 'js', 'ts',
  'py', 'java', 'go', 'cpp', 'c', 'h', 'sh', 'yaml', 'yml',
  'ini', 'conf', 'log', 'sql', 'rs', 'rb', 'php', 'swift', 'kt',
  'scala', 'r', 'lua', 'pl', 'toml',
]);

const canPreview = (): boolean => {
  if (props.details?.type !== 'file') return false;
  const ft = props.details?.file_type?.toLowerCase();
  return !!ft && previewSupportedTypes.has(ft);
};

// 瑜版挻鏋冨锝堫嚊閹懎濮炴潪钘夌暚閹存劖妞傞敍瀹杋le 缁鐎烽懛顏勫З閸掑洦宕查崚鑸偓宀勵暕鐟欏牄鈧?
watch(() => props.details?.id, (newId) => {
  if (!newId) return;
  viewMode.value = 'merged';
});

const isTextFile = (fileType?: string): boolean => {
  if (!fileType) return false;
  const textTypes = ['txt', 'md', 'markdown', 'json', 'xml', 'html', 'css', 'js', 'ts', 'py', 'java', 'go', 'cpp', 'c', 'h', 'sh', 'yaml', 'yml', 'ini', 'conf', 'log'];
  return textTypes.includes(fileType.toLowerCase());
};
const isMarkdownFile = (fileType?: string): boolean => {
  if (!fileType) return false;
  const markdownTypes = ['md', 'markdown'];
  return markdownTypes.includes(fileType.toLowerCase());
};
const runMarkdownPostRenderPipeline = async () => {
  await nextTick();
  const renderRoot = mdContentWrap.value as ParentNode;
  await hydrateProtectedFileImages(renderRoot);
  const images = renderRoot?.querySelectorAll?.('img.markdown-image') as NodeListOf<HTMLImageElement> | undefined;
  if (images) {
    images.forEach(async item => {
      const isValid = await checkImage(item.src);
      if (!isValid) {
        item.remove();
      }
    })
  }
  await renderMermaidInContainer(mdContentWrap.value);
};

watch(() => props.details.md, async () => {
  await refreshRenderedMarkdown();
}, { immediate: true, deep: true })

watch(() => viewMode.value, async (mode) => {
  if ((mode === 'chunks' || mode === 'merged') && props.visible) {
    await refreshRenderedMarkdown();
  }
});

watch(() => props.visible, async (visible) => {
  if (visible && (viewMode.value === 'chunks' || viewMode.value === 'merged')) {
    await refreshRenderedMarkdown();
  }
});

// 鐎瑰鍙忛崷鏉款槱閻?Markdown 閸愬懎顔愰敍鍫滃▏閻?marked閿?

const handleClose = () => {
  emit("closeDoc", false);
  if (doc) {
    doc.scrollTop = 0;
  }
  viewMode.value = 'merged';
};

// 閼惧嘲褰囬弰鍓с仛閺嶅洭顣?
const getDisplayTitle = () => {
  if (!props.details.title) return '';
  if (props.details.type === 'file') {
    // 閺傚洣娆㈢猾璇茬€烽崢缁樺竴閹碘晛鐫嶉崥?
    const lastDotIndex = props.details.title.lastIndexOf(".");
    return lastDotIndex > 0 ? props.details.title.substring(0, lastDotIndex) : props.details.title;
  }
  // URL閸滃本澧滈崝銊ュ灡瀵よ櫣娲块幒銉ㄧ箲閸ョ偞鐖ｆ０?
  return props.details.title;
};

const channelLabelMap: Record<string, string> = {
  web: 'knowledgeBase.channelWeb',
  api: 'knowledgeBase.channelApi',
  browser_extension: 'knowledgeBase.channelBrowserExtension',
  wechat: 'knowledgeBase.channelWechat',
  wecom: 'knowledgeBase.channelWecom',
  feishu: 'knowledgeBase.channelFeishu',
  dingtalk: 'knowledgeBase.channelDingtalk',
  slack: 'knowledgeBase.channelSlack',
  im: 'knowledgeBase.channelIm',
};

const getChannelLabel = (channel: string) => {
  const key = channelLabelMap[channel];
  return key ? t(key) : t('knowledgeBase.channelUnknown');
};

// 閼惧嘲褰囩猾璇茬€烽弽鍥╊劮
const getTypeLabel = () => {
  switch (props.details.type) {
    case 'url':
      return t('knowledgeBase.typeURL');
    case 'manual':
      return t('knowledgeBase.typeManual');
    case 'file':
      return props.details.file_type ? props.details.file_type.toUpperCase() : t('knowledgeBase.typeFile');
    default:
      return '';
  }
};

// 閼惧嘲褰囩猾璇茬€锋稉濠氼暯閼?
const getTypeTheme = () => {
  switch (props.details.type) {
    case 'url':
      return 'primary';
    case 'manual':
      return 'success';
    case 'file':
      return 'default';
    default:
      return 'default';
  }
};

// 閼惧嘲褰囬崘鍛啇閺嶅洨顒?
const getContentLabel = () => {
  switch (props.details.type) {
    case 'url':
      return t('knowledgeBase.webContent');
    case 'manual':
      return t('knowledgeBase.documentContent');
    case 'file':
    default:
      return t('knowledgeBase.fileContent');
  }
};

// 閼惧嘲褰囬弮鍫曟？閺嶅洨顒?
const getTimeLabel = () => {
  switch (props.details.type) {
    case 'url':
      return t('knowledgeBase.importTime');
    case 'manual':
      return t('knowledgeBase.createTime');
    case 'file':
    default:
      return t('knowledgeBase.uploadTime');
  }
};

// 閼惧嘲褰嘋hunk閺嶅嘲绱＄猾?
const normalizeChunkIndex = (index: string | number) => Number(index) || 0;
const getChunkDisplayIndex = (index: string | number) => normalizeChunkIndex(index) + 1;

const getChunkClass = (index: string | number) => {
  return normalizeChunkIndex(index) % 2 !== 0 ? 'chunk-odd' : 'chunk-even';
};

// 閼惧嘲褰嘋hunk閸忓啯鏆熼幑?
const getChunkMeta = (item: any) => {
  if (!item) return '';
  const parts = [];
  if (item.char_count) {
    parts.push(`${item.char_count} ${t('knowledgeBase.characters')}`);
  }
  if (item.token_count) {
    parts.push(`${item.token_count} tokens`);
  }
  return parts.join(' · ');
};

// 閻㈢喐鍨氶惃鍕６妫版琚崹?
interface GeneratedQuestion {
  id: string;
  question: string;
}

// 鐟欙絾鐎介悽鐔稿灇閻ㄥ嫰妫舵０?
const getGeneratedQuestions = (item: any): GeneratedQuestion[] => {
  if (!item || !item.metadata) return [];
  try {
    const metadata = typeof item.metadata === 'string' ? JSON.parse(item.metadata) : item.metadata;
    const questions = metadata.generated_questions || [];
    // 閸忕厧顔愰弮褎鐗稿蹇ョ礄鐎涙顑佹稉鍙夋殶缂佸嫸绱氶崪灞炬煀閺嶇厧绱￠敍鍫濐嚠鐠炩剝鏆熺紒鍕剁礆
    return questions.map((q: string | GeneratedQuestion, index: number) => {
      if (typeof q === 'string') {
        // 閺冄勭壐瀵骏绱扮€涙顑佹稉璇х礉閻㈢喐鍨氭稉瀛樻ID
        return { id: `legacy-${index}`, question: q };
      }
      return q;
    });
  } catch {
    return [];
  }
};

// 鐏炴洖绱戦悩鑸碘偓浣侯吀閻?
const expandedChunks = ref<Set<string | number>>(new Set());

const toggleQuestions = (index: string | number) => {
  if (expandedChunks.value.has(index)) {
    expandedChunks.value.delete(index);
  } else {
    expandedChunks.value.add(index);
  }
  // 鐟欙箑褰傞崫宥呯安瀵繑娲块弬?
  expandedChunks.value = new Set(expandedChunks.value);
};

const isExpanded = (index: string | number) => expandedChunks.value.has(index);

// 閸掔娀娅庢稉顓犳畱閻樿埖鈧?
const deletingQuestion = ref<{ chunkIndex: string | number; questionId: string } | null>(null);

// 閸掔娀娅庨悽鐔稿灇閻ㄥ嫰妫舵０?
const handleDeleteQuestion = async (item: any, chunkIndex: string | number, question: GeneratedQuestion) => {
  if (!item || !item.id) {
    MessagePlugin.error(t('common.error'));
    return;
  }

  // 濡偓閺屻儲妲搁崥锔芥Ц閺冄勭壐瀵繑鏆熼幑顕嗙礄閺冪姵纭堕崚鐘绘珟閿?
  if (question.id.startsWith('legacy-')) {
    MessagePlugin.warning(t('knowledgeBase.legacyQuestionCannotDelete'));
    return;
  }

  const confirmDialog = DialogPlugin.confirm({
    header: t('common.confirmDelete'),
    body: t('knowledgeBase.confirmDeleteQuestion'),
    confirmBtn: t('common.confirm'),
    cancelBtn: t('common.cancel'),
    onConfirm: async () => {
      confirmDialog.hide();
      deletingQuestion.value = { chunkIndex, questionId: question.id };
      try {
        await deleteGeneratedQuestion(item.id, question.id);
        MessagePlugin.success(t('common.deleteSuccess'));
        
        // 閺囧瓨鏌婇張顒€婀撮弫鐗堝祦
        const metadata = typeof item.metadata === 'string' ? JSON.parse(item.metadata) : item.metadata;
        if (metadata && metadata.generated_questions) {
          const idx = metadata.generated_questions.findIndex((q: GeneratedQuestion) => q.id === question.id);
          if (idx > -1) {
            metadata.generated_questions.splice(idx, 1);
          }
          item.metadata = typeof item.metadata === 'string' ? JSON.stringify(metadata) : metadata;
        }
        
        // 闁氨鐓￠悥鍓佺矋娴犺泛鍩涢弬鐗堟殶閹?
        emit('questionDeleted', { chunkId: item.id, questionId: question.id });
      } catch (error: any) {
        MessagePlugin.error(error?.message || t('common.deleteFailed'));
      } finally {
        deletingQuestion.value = null;
      }
    },
    onClose: () => {
      confirmDialog.hide();
    }
  });
};

// 濡偓閺屻儲妲搁崥锔筋劀閸︺劌鍨归梽銈嗙厙娑擃亪妫舵０?
const isDeleting = (chunkIndex: string | number, questionId: string) => {
  return deletingQuestion.value?.chunkIndex === chunkIndex && deletingQuestion.value?.questionId === questionId;
};

// 閻?Chunk 娑撳﹣绗呴弬鍥х潔瀵偓閻樿埖鈧?
const parentContextExpanded = ref<Set<string | number>>(new Set());
const parentContextCache = ref<Map<string, string>>(new Map());
const parentContextLoading = ref<Set<string | number>>(new Set());

const hasParentChunk = (item: any) => !!item?.parent_chunk_id;

const isParentExpanded = (index: string | number) => parentContextExpanded.value.has(index);

const toggleParentContext = async (item: any, index: string | number) => {
  if (parentContextExpanded.value.has(index)) {
    parentContextExpanded.value.delete(index);
    parentContextExpanded.value = new Set(parentContextExpanded.value);
    return;
  }
  
  const parentId = item.parent_chunk_id;
  if (!parentContextCache.value.has(parentId)) {
    parentContextLoading.value.add(index);
    parentContextLoading.value = new Set(parentContextLoading.value);
    try {
      const result: any = await getChunkByIdOnly(parentId);
      if (result.success && result.data) {
        parentContextCache.value.set(parentId, result.data.content || '');
        parentContextCache.value = new Map(parentContextCache.value);
      }
    } catch (err) {
      MessagePlugin.error(t('knowledgeBase.parentContextLoadFailed'));
      return;
    } finally {
      parentContextLoading.value.delete(index);
      parentContextLoading.value = new Set(parentContextLoading.value);
    }
  }
  
  parentContextExpanded.value.add(index);
  parentContextExpanded.value = new Set(parentContextExpanded.value);
  await refreshRenderedMarkdown();
};

const getParentContent = (item: any) => {
  return parentContextCache.value.get(item.parent_chunk_id) || '';
};

const downloadFile = () => {
  downKnowledgeDetails(props.details.id)
    .then((result) => {
      if (result) {
        if (url.value) {
          URL.revokeObjectURL(url.value);
        }
        url.value = URL.createObjectURL(result);
        const link = document.createElement("a");
        link.style.display = "none";
        link.setAttribute("href", url.value);
        const needsExt = props.details.type === 'manual' && !props.details.title.toLowerCase().endsWith('.md');
        const ext = needsExt ? '.md' : '';
        link.setAttribute("download", props.details.title + ext);
        document.body.appendChild(link);
        link.click();
        nextTick(() => {
          document.body.removeChild(link);
          URL.revokeObjectURL(url.value);
        })
      }
    })
    .catch((err) => {
      MessagePlugin.error(t('file.downloadFailed'));
    });
};
const handleDetailsScroll = () => {
  if (doc && !loadingChunks) {
    let pageNum = Math.ceil(props.details.total / 25);
    const { scrollTop, scrollHeight, clientHeight } = doc;
    if (scrollTop + clientHeight >= scrollHeight - 8) {
      if (props.details.md.length < props.details.total && page + 1 <= pageNum) {
        page++;
        loadingChunks = true;
        pendingRequestedPage = page;
        pendingChunksBeforeLoad = props.details.md.length;
        emit("getDoc", page);
      }
    }
  }
};
</script>
<template>
  <div class="doc_content" ref="mdContentWrap">
    <t-drawer :visible="visible" :zIndex="2000" :closeBtn="true" @close="handleClose">
      <template #header>
        <div class="drawer-header">
          <span class="header-title">{{ getDisplayTitle() }}</span>
          <t-tag v-if="details.type" size="small" :theme="getTypeTheme()" variant="light">
            {{ getTypeLabel() }}
          </t-tag>
        </div>
      </template>
      
      <!-- 閺傚洣娆㈢猾璇茬€锋稉鎾崇潣閸栧搫鐓?-->
      <div v-if="details.type === 'file'" class="doc_box">
        <a :href="url" style="display: none" ref="down" :download="details.title"></a>
        <span class="label">{{ $t('knowledgeBase.fileName') }}</span>
        <div class="download_box">
          <span class="doc_t">{{ details.title }}</span>
          <div class="icon_box" @click="downloadFile()" aria-label="Download">
            <img class="download_box" src="@/assets/img/download.svg" alt="">
          </div>
        </div>
      </div>
      
      <!-- URL缁鐎锋稉鎾崇潣閸栧搫鐓?-->
      <div v-else-if="details.type === 'url'" class="url_box">
        <span class="label">{{ $t('knowledgeBase.urlSource') }}</span>
        <div class="url_link_box">
          <a :href="details.source" target="_blank" class="url_link">
            <t-icon name="link" size="14px" />
            <span class="url_text">{{ details.source }}</span>
            <t-icon name="jump" size="14px" class="jump-icon" />
          </a>
        </div>
      </div>
      
      <!-- 閹靛濮╅崚娑樼紦缁鐎锋稉鎾崇潣閸栧搫鐓?-->
      <div v-else-if="details.type === 'manual'" class="manual_box">
        <span class="label">{{ $t('knowledgeBase.documentTitle') }}</span>
        <div class="download_box">
          <div class="manual_title_box">
            <span class="manual_title">{{ details.title }}</span>
          </div>
          <div class="icon_box" @click="downloadFile()" aria-label="Download">
            <img class="download_box" src="@/assets/img/download.svg" alt="">
          </div>
        </div>
      </div>
      
      <div class="content_header">
        <div class="header-left">
          <div class="title-row">
            <span class="label">{{ getContentLabel() }}</span>
            <span v-if="details.total > 0" class="chunk-count">
              {{ $t('knowledgeBase.chunkCount', { count: details.total }) }}
            </span>
          </div>
          <div class="meta-row">
            <span class="time"> {{ getTimeLabel() }}：{{ details.time }} </span>
            <t-tag v-if="details.channel && details.channel !== 'web'" size="small" variant="light" theme="warning" class="channel-tag">
              {{ getChannelLabel(details.channel) }}
            </t-tag>
            <div class="view-mode-buttons">
              <t-button 
                v-if="canPreview()"
                size="small" 
                :variant="viewMode === 'preview' ? 'base' : 'outline'" 
                :theme="viewMode === 'preview' ? 'primary' : 'default'"
                @click="viewMode = 'preview'"
                class="view-mode-btn"
              >
                {{ $t('preview.tab') }}
              </t-button>
              <t-button 
                size="small" 
                :variant="viewMode === 'merged' ? 'base' : 'outline'" 
                :theme="viewMode === 'merged' ? 'primary' : 'default'"
                @click="viewMode = 'merged'"
                class="view-mode-btn"
              >
                {{ $t('knowledgeBase.viewMerged') }}
              </t-button>
              <t-button 
                size="small" 
                :variant="viewMode === 'chunks' ? 'base' : 'outline'" 
                :theme="viewMode === 'chunks' ? 'primary' : 'default'"
                @click="viewMode = 'chunks'"
                class="view-mode-btn"
              >
                {{ $t('knowledgeBase.viewChunks') }}
              </t-button>
            </div>
          </div>
        </div>
      </div>
      
      <!-- 閸氬牆鑻熺憴鍡楁禈 -->
      <div v-if="viewMode === 'merged'">
        <div v-if="!mergedContent" class="no_content">{{ $t('common.noData') }}</div>
        <div v-else class="md-content" v-html="renderedMergedContent"></div>
      </div>
      
      <!-- 閸掑棗娼＄憴鍡楁禈 -->
      <div v-else-if="viewMode === 'chunks'">
        <div v-if="details.md.length == 0" class="no_content">{{ $t('common.noData') }}</div>
        <div v-else class="chunk-list">
          <div class="chunk-item" 
            v-for="(item, index) in details.md" 
            :key="index"
            :class="getChunkClass(index)"
          >
            <div class="chunk-header">
              <span class="chunk-index">{{ $t('knowledgeBase.segment') }} {{ getChunkDisplayIndex(index) }}</span>
              <div class="chunk-header-right">
                <t-tag 
                  v-if="hasParentChunk(item)" 
                  size="small" 
                  theme="primary" 
                  variant="light"
                >
                  {{ $t('knowledgeBase.childChunk') }}
                </t-tag>
                <t-tag 
                  v-if="getGeneratedQuestions(item).length > 0" 
                  size="small" 
                  theme="success" 
                  variant="light"
                >
                  {{ $t('knowledgeBase.questions') }} {{ getGeneratedQuestions(item).length }}
                </t-tag>
                <span class="chunk-meta">{{ getChunkMeta(item) }}</span>
              </div>
            </div>
            <div class="md-content" v-html="getRenderedChunkHTML(item.content)"></div>
            
            <!-- 閻?Chunk 娑撳﹣绗呴弬鍥х潔瀵偓 -->
            <div v-if="hasParentChunk(item)" class="parent-context-section">
              <div class="parent-context-toggle" @click="toggleParentContext(item, index)">
                <t-icon v-if="!parentContextLoading.has(index)" :name="isParentExpanded(index) ? 'chevron-down' : 'chevron-right'" size="14px" />
                <t-loading v-else size="small" style="width: 14px; height: 14px;" />
                <span>{{ $t('knowledgeBase.viewParentContext') }}</span>
              </div>
              <div v-show="isParentExpanded(index)" class="parent-context-content">
                <div class="md-content" v-html="getRenderedParentHTML(item)"></div>
              </div>
            </div>
            
            <!-- 閻㈢喐鍨氶惃鍕６妫版ê鐫嶇粈?-->
            <div v-if="getGeneratedQuestions(item).length > 0" class="questions-section">
              <div class="questions-toggle" @click="toggleQuestions(index)">
                <t-icon :name="isExpanded(index) ? 'chevron-down' : 'chevron-right'" size="14px" />
                <span>{{ $t('knowledgeBase.generatedQuestions') }} ({{ getGeneratedQuestions(item).length }})</span>
              </div>
              <div v-show="isExpanded(index)" class="questions-list">
                <div 
                  v-for="question in getGeneratedQuestions(item)" 
                  :key="question.id" 
                  class="question-item"
                >
                  <t-icon name="help-circle" size="14px" class="question-icon" />
                  <span class="question-text">{{ question.question }}</span>
                  <t-button 
                    theme="default" 
                    variant="text" 
                    size="small"
                    class="delete-question-btn"
                    :loading="isDeleting(index, question.id)"
                    @click.stop="handleDeleteQuestion(item, index, question)"
                  >
                    <template #icon>
                      <t-icon name="delete" size="14px" />
                    </template>
                  </t-button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
      
      <!-- 閺傚洦銆傛０鍕潔鐟欏棗娴?-->
      <div v-else-if="viewMode === 'preview'">
        <DocumentPreview
          :knowledgeId="details.id"
          :fileType="details.file_type"
          :fileName="details.title"
          :active="viewMode === 'preview'"
        />
      </div>
      
      <template #footer>
        <t-button @click="handleClose">{{ $t('common.confirm') }}</t-button>
        <t-button theme="default" @click="handleClose">{{ $t('common.cancel') }}</t-button>
      </template>
    </t-drawer>
  </div>
</template>
<style scoped lang="less">
@import "./css/markdown.less";

:deep(.t-drawer .t-drawer__content-wrapper) {
  width: min(654px, 85vw) !important; // 閸戝繐鐨崚?5%鐟欏棗褰涚€硅棄瀹抽敍宀€绮板锔挎櫠閻ｆ瑦娲挎径姘扁敄闂?
  max-width: 654px !important;
}

// 閸︺劌鐨仦蹇撶娑撳﹨绻樻稉鈧銉ㄧ殶閺?
@media (max-width: 768px) {
  :deep(.t-drawer .t-drawer__content-wrapper) {
    width: 90vw !important; // 鐏忓繐鐫嗛獮鏇氱瑐娴ｈ法鏁?0%鐎硅棄瀹?
    max-width: none !important;
  }
}

// 娴狅絿鐖滈崸妤佺壉瀵?
:deep(.code-block-wrapper) {
  margin: 12px 0;
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  overflow: hidden;
  box-shadow: 0 1px 2px rgba(0,0,0,0.05);

  .code-block-header {
    display: flex;
    align-items: center;
    padding: 8px 12px;
    background: var(--td-bg-color-secondarycontainer);
    border-bottom: 1px solid var(--td-component-stroke);
    font-size: 12px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  .code-block-pre {
    margin: 0;
    padding: 12px;
    background: var(--td-bg-color-secondarycontainer);
    overflow: auto;
    font-size: 13px;
    line-height: 1.5;
    code {
      background: transparent;
      padding: 0;
      border: none;
      white-space: pre;
      word-wrap: normal;
      display: block;
    }
  }
}

:deep(.t-drawer__header) {
  font-weight: 800;
}

:deep(.t-drawer__body.narrow-scrollbar) {
  padding: 16px 24px;
}

.drawer-header {
  display: flex;
  align-items: center;
  gap: 12px;
  
  .header-title {
    flex: 1;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.doc_box, .url_box, .manual_box {
  display: flex;
  flex-direction: column;
  margin-bottom: 16px;
}

.label {
  color: var(--td-text-color-primary);
  font-size: 14px;
  font-style: normal;
  font-weight: 500;
  line-height: 22px;
  margin-bottom: 8px;
}

// 閺傚洣娆㈡稉瀣祰閸栧搫鐓?
.download_box {
  display: flex;
  align-items: center;
}

.doc_t {
  box-sizing: border-box;
  display: flex;
  padding: 5px 8px;
  align-items: center;
  border-radius: 3px;
  border: 1px solid var(--td-component-border);
  background: var(--td-bg-color-container-hover);
  word-break: break-all;
  text-align: justify;
}

.icon_box {
  margin-left: 18px;
  display: flex;
  overflow: hidden;
  color: var(--td-brand-color);

  .download_box {
    width: 16px;
    height: 16px;
    fill: currentColor;
    overflow: hidden;
    cursor: pointer;
  }
}

// URL闁剧偓甯撮崠鍝勭厵
.url_link_box {
  border-radius: 4px;
  border: 1px solid var(--td-success-color-focus);
  background: var(--td-success-color-light);
  padding: 8px 12px;
  
  .url_link {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--td-brand-color-active);
    text-decoration: none;
    transition: all 0.2s ease;
    
    &:hover {
      color: var(--td-brand-color);
      background: var(--td-success-color-light);
      border-radius: 3px;
      padding: 4px 6px;
      margin: -4px -6px;
      
      .jump-icon {
        transform: translateX(2px);
      }
    }
    
    .url_text {
      flex: 1;
      font-size: 13px;
      word-break: break-all;
    }
    
    .jump-icon {
      transition: transform 0.2s ease;
      flex-shrink: 0;
      color: var(--td-brand-color-active);
    }
  }
}

// 閹靛濮╅崚娑樼紦閺嶅洭顣介崠鍝勭厵
.manual_title_box {
  border-radius: 4px;
  border: 1px solid var(--td-component-border);
  background: var(--td-bg-color-container-hover);
  padding: 8px 12px;
  
  .manual_title {
    color: var(--td-text-color-primary);
    font-size: 14px;
    font-weight: 500;
    word-break: break-word;
  }
}

.content_header {
  margin-top: 22px;
  margin-bottom: 16px;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;

  .header-left {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .title-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .meta-row {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }

  .channel-tag {
    flex-shrink: 0;
  }

  .chunk-count {
    color: var(--td-brand-color);
    font-size: 12px;
    background: var(--td-brand-color)14;
    padding: 4px 8px;
    border-radius: 12px;
  }

  .view-mode-buttons {
    display: flex;
    gap: 4px;
    
    .view-mode-btn {
      height: 28px;
      min-width: 60px;
    }
  }

  .view-mode-toggle {
    height: 28px;
  }
}

.time {
  color: var(--td-text-color-disabled);
  font-size: 12px;
  font-style: normal;
  font-weight: 400;
  line-height: 20px;
}

.no_content {
  margin-top: 12px;
  color: var(--td-text-color-disabled);
  font-size: 12px;
  padding: 16px;
  background: var(--td-bg-color-container);
  text-align: center;
}

// Chunk閸掓銆冮弽宄扮础
.chunk-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.chunk-item {
  border-radius: 6px;
  padding: 12px;
  transition: all 0.2s ease;
  border: 1px solid transparent;
  
  &.chunk-even {
    background: var(--td-bg-color-container-hover);
  }
  
  &.chunk-odd {
    background: var(--td-brand-color)0d;
  }
  
  &:hover {
    border-color: var(--td-brand-color);
    box-shadow: 0 2px 8px rgba(7, 192, 95, 0.1);
  }
}

.chunk-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--td-component-stroke);
  
  .chunk-index {
    color: var(--td-text-color-placeholder);
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.5px;
  }
  
  .chunk-header-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  
  .chunk-meta {
    color: var(--td-text-color-disabled);
    font-size: 11px;
  }
}

// 閻?Chunk 娑撳﹣绗呴弬鍥ㄧ壉瀵?
.parent-context-section {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px dashed var(--td-component-stroke);
}

.parent-context-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  color: var(--td-brand-color);
  font-size: 12px;
  font-weight: 500;
  padding: 4px 0;
  transition: color 0.2s ease;
  
  &:hover {
    color: var(--td-brand-color);
  }
}

.parent-context-content {
  margin-top: 8px;
  padding: 10px 12px;
  background: var(--td-brand-color-light);
  border-radius: 4px;
  border-left: 3px solid var(--td-brand-color);
  
  .md-content {
    color: var(--td-text-color-secondary);
    font-size: 13px;
  }
}

// 閻㈢喐鍨氶惃鍕６妫版ɑ鐗卞?
.questions-section {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px dashed var(--td-component-stroke);
}

.questions-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  color: var(--td-brand-color-active);
  font-size: 12px;
  font-weight: 500;
  padding: 4px 0;
  transition: color 0.2s ease;
  
  &:hover {
    color: var(--td-brand-color);
  }
}

.questions-list {
  margin-top: 8px;
  padding-left: 4px;
}

.question-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 8px;
  margin-bottom: 4px;
  background: var(--td-success-color-light);
  border-radius: 4px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  line-height: 1.5;
  transition: background-color 0.2s ease;
  
  &:hover {
    background: var(--td-success-color-light);
    
    .delete-question-btn {
      opacity: 1;
    }
  }
  
  .question-icon {
    color: var(--td-brand-color-active);
    flex-shrink: 0;
    margin-top: 2px;
  }
  
  .question-text {
    flex: 1;
    word-break: break-word;
  }
  
  .delete-question-btn {
    opacity: 0;
    flex-shrink: 0;
    color: var(--td-text-color-placeholder);
    transition: opacity 0.2s ease, color 0.2s ease;
    
    &:hover {
      color: var(--td-error-color);
    }
  }
}

.md-content {
  word-break: break-word;
  line-height: 1.6;
  color: var(--td-text-color-primary);
}

// 娣囨繄鏆€閺冄勭壉瀵繋缍旀稉鍝勫悑鐎圭櫢绱欏鑼额潶chunk-item閺囧じ鍞敍?
.content {
  word-break: break-word;
  padding: 4px;
  gap: 4px;
  margin-top: 12px;
}
</style>
