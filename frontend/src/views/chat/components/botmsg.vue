<template>
    <div class="bot_msg">
        <div style="display: flex;flex-direction: column; gap:8px">
            <!-- 鏄剧ず@鐨勭煡璇嗗簱鍜屾枃浠讹紙闈?Agent 妯″紡涓嬫樉绀猴級 -->
            <div v-if="!session.isAgentMode && mentionedItems && mentionedItems.length > 0" class="mentioned_items">
                <span
                    v-for="item in mentionedItems"
                    :key="item.id"
                    class="mentioned_tag"
                    :class="[
                      item.type === 'kb' ? (item.kb_type === 'faq' ? 'faq-tag' : 'kb-tag') : 'file-tag'
                    ]"
                >
                    <span class="tag_icon">
                        <t-icon v-if="item.type === 'kb'" :name="item.kb_type === 'faq' ? 'chat-bubble-help' : 'folder'" />
                        <t-icon v-else name="file" />
                    </span>
                    <span class="tag_name">{{ item.name }}</span>
                </span>
            </div>
            <docInfo :session="session"></docInfo>
            <ConfidencePanel
                :message-id="session?.id"
                :is-completed="session?.is_completed"
                :reference-count="session?.knowledge_references?.length || 0"
            />
            <AgentStreamDisplay :session="session" :user-query="userQuery" :session-id="sessionId" v-if="session.isAgentMode"></AgentStreamDisplay>
            <deepThink :deepSession="session" v-if="session.showThink && !session.isAgentMode"></deepThink>
        </div>
        <!-- 闈?Agent 妯″紡涓嬫墠鏄剧ず浼犵粺鐨?markdown 娓叉煋 -->
        <div ref="parentMd" v-if="!session.hideContent && !session.isAgentMode">
            <!-- 鐩存帴娓叉煋瀹屾暣鍐呭锛岄伩鍏嶅垏鍒嗗鑷寸殑闂锛屾牱寮忎笌 thinking 涓€鑷?-->
            <!-- 鍙湁褰撴湁瀹為檯鍐呭鏃舵墠鏄剧ず鍖呭洿妗?-->
            <div class="content-wrapper" v-if="hasActualContent">
                <div class="ai-markdown-template markdown-content">
                    <div v-for="(tokenHtml, index) in renderedMarkdownTokens" :key="index" v-html="tokenHtml"></div>
                </div>
            </div>
            <!-- Streaming indicator (non-Agent mode) -->
            <div v-if="hasActualContent && !session.is_completed" class="loading-indicator">
                <div class="loading-typing">
                    <span></span>
                    <span></span>
                    <span></span>
                </div>
            </div>
            <!-- 澶嶅埗鍜屾坊鍔犲埌鐭ヨ瘑搴撴寜閽?- 闈?Agent 妯″紡涓嬫樉绀?-->
            <div v-if="session.is_completed && (content || session.content)" class="answer-toolbar">
                <t-button size="small" variant="outline" shape="round" @click.stop="handleCopyAnswer" :title="$t('agent.copy')">
                    <t-icon name="copy" />
                </t-button>
                <t-button size="small" variant="outline" shape="round" @click.stop="handleAddToKnowledge" :title="$t('agent.addToKnowledgeBase')">
                    <t-icon name="add" />
                </t-button>
                <!-- 鐐硅禐/韪╁弽棣?-->
                <t-tooltip :content="$t('chat.feedbackLike')" placement="top">
                    <t-button
                        size="small" variant="outline" shape="round"
                        :class="['feedback-btn', { 'feedback-active-like': localFeedback === 'like' }]"
                        @click.stop="handleFeedback('like')"
                    >
                        <t-icon name="thumb-up" />
                    </t-button>
                </t-tooltip>
                <t-tooltip :content="$t('chat.feedbackDislike')" placement="top">
                    <t-button
                        size="small" variant="outline" shape="round"
                        :class="['feedback-btn', { 'feedback-active-dislike': localFeedback === 'dislike' }]"
                        @click.stop="handleFeedback('dislike')"
                    >
                        <t-icon name="thumb-down" />
                    </t-button>
                </t-tooltip>
                <!-- Fallback 鎻愮ず鍥炬爣 -->
                <t-tooltip v-if="session.is_fallback" :content="$t('chat.fallbackHint')" placement="top">
                    <t-button size="small" variant="outline" shape="round" class="fallback-icon-btn">
                        <t-icon name="info-circle" />
                    </t-button>
                </t-tooltip>
                <!-- 閲嶆柊鐢熸垚 -->
                <t-tooltip :content="$t('chat.regenerate')" placement="top">
                    <t-button size="small" variant="outline" shape="round" @click.stop="emit('regenerate', userQuery)">
                        <t-icon name="refresh" />
                    </t-button>
                </t-tooltip>
            </div>
            <div v-if="isImgLoading" class="img_loading"><t-loading size="small"></t-loading><span>{{ $t('common.loading') }}</span></div>
        </div>
        <!-- Agent 妯″紡宸ュ叿鏍忥紙閲嶆柊鐢熸垚锛?->
        <div v-if="session.isAgentMode && session.is_completed" class="answer-toolbar agent-toolbar">
            <t-tooltip :content="$t('chat.regenerate')" placement="top">
                <t-button size="small" variant="outline" shape="round" @click.stop="emit('regenerate', userQuery)">
                    <t-icon name="refresh" />
                </t-button>
            </t-tooltip>
        </div>
        <!-- 绛斿悗杩介棶鎺ㄨ崘 -->
        <div v-if="session.is_completed && isLatest && followUpQuestions.length > 0" class="follow-up-section">
            <div class="follow-up-label">{{ $t('chat.followUpQuestions') }}</div>
            <div class="follow-up-chips">
                <span
                    v-for="item in followUpQuestions"
                    :key="item.question"
                    class="follow-up-chip"
                    @click="emit('send-question', item.question)"
                >{{ item.question }}</span>
            </div>
        </div>
        <picturePreview :reviewImg="reviewImg" :reviewUrl="reviewUrl" @closePreImg="closePreImg"></picturePreview>
    </div>
</template>
<script setup>
import { defineAsyncComponent, onMounted, onBeforeUnmount, watch, computed, ref, reactive, defineProps, nextTick, onUpdated } from 'vue';
import { sanitizeHTML, safeMarkdownToHTML, createSafeImage, isValidImageURL, hydrateProtectedFileImages } from '@/utils/security';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next/es/message';
import { useUIStore } from '@/stores/ui';
import { useSettingsStore } from '@/stores/settings';
import { submitMessageFeedback } from '@/api/chat/index';
import { getSuggestedQuestions } from '@/api/agent/index';
import { normalizeSuggestedQuestions } from '@/utils/suggestedQuestions';
import {
    buildManualMarkdown,
    copyTextToClipboard,
    formatManualTitle,
    replaceIncompleteImageWithPlaceholder
} from '@/utils/chatMessageShared';
import {
    createMermaidCodeRenderer,
    renderMermaidInContainer
} from '@/utils/mermaidShared';

const docInfo = defineAsyncComponent(() => import('./docInfo.vue'));
const ConfidencePanel = defineAsyncComponent(() => import('./ConfidencePanel.vue'));
const deepThink = defineAsyncComponent(() => import('./deepThink.vue'));
const AgentStreamDisplay = defineAsyncComponent(() => import('./AgentStreamDisplay.vue'));
const picturePreview = defineAsyncComponent(() => import('@/components/picture-preview.vue'));

const emit = defineEmits(['scroll-bottom', 'regenerate', 'send-question'])
const { t } = useI18n()
const uiStore = useUIStore();
let parentMd = ref()
let reviewUrl = ref('')
let reviewImg = ref(false)
let isImgLoading = ref(false);
const props = defineProps({
    // 蹇呭～椤?
    content: {
        type: String,
        required: false
    },
    session: {
        type: Object,
        required: false
    },
    userQuery: {
        type: String,
        required: false,
        default: ''
    },
    isFirstEnter: {
        type: Boolean,
        required: false
    },
    sessionId: {
        type: [String, Object],
        required: false,
        default: ''
    },
    isLatest: {
        type: Boolean,
        required: false,
        default: false
    }
});

// 鏈湴鍙嶉鐘舵€侊細鍒濆鍖栨椂浠?session.feedback 璇诲彇
const localFeedback = ref(props.session?.feedback || '');

// 绛斿悗杩介棶
const useSettingsStoreInstance = useSettingsStore();
const followUpQuestions = ref([]);
const followUpLoading = ref(false);
const renderedMarkdownTokens = ref([]);
let markedInstancePromise = null;
let customRenderer = null;
let markdownRenderVersion = 0;

const fetchFollowUp = async () => {
    const agentId = useSettingsStoreInstance.selectedAgentId;
    if (!agentId || followUpLoading.value || followUpQuestions.value.length > 0) return;
    followUpLoading.value = true;
    try {
        const selectedKBs = useSettingsStoreInstance.getSelectedKnowledgeBases();
        const selectedFiles = useSettingsStoreInstance.getSelectedFiles();
        const res = await getSuggestedQuestions(agentId, {
            knowledge_base_ids: selectedKBs.length > 0 ? selectedKBs : undefined,
            knowledge_ids: selectedFiles.length > 0 ? selectedFiles : undefined,
            limit: 3,
        });
        followUpQuestions.value = normalizeSuggestedQuestions(res?.data?.questions, 3);
    } catch {
        // silently ignore
    } finally {
        followUpLoading.value = false;
    }
};

const loadMarked = async () => {
    if (!markedInstancePromise) {
        markedInstancePromise = import('marked').then(async ({ marked }) => {
            marked.use({
                breaks: true,
            });

            if (!customRenderer) {
                customRenderer = new marked.Renderer();
                customRenderer.image = function({ href, title, text }) {
                    if (!isValidImageURL(href)) {
                        return `<p>${t('error.invalidImageLink')}</p>`;
                    }
                    return createSafeImage(href, text || '', title || '');
                };
                customRenderer.code = await createMermaidCodeRenderer('mermaid-botmsg');
            }

            return marked;
        });
    }

    return markedInstancePromise;
};

const renderMarkdownTokens = async () => {
    const renderVersion = ++markdownRenderVersion;
    const text = props.content || props.session?.content || '';

    if (!text || typeof text !== 'string' || !text.trim()) {
        renderedMarkdownTokens.value = [];
        return;
    }

    try {
        const marked = await loadMarked();
        const processed = replaceIncompleteImageWithPlaceholder(text);
        const safeMarkdown = safeMarkdownToHTML(processed);
        const tokens = marked.lexer(safeMarkdown);
        const tokenHtmlList = tokens.map((token) => {
            const html = marked.parser([token], {
                renderer: customRenderer,
                breaks: true,
            });
            return sanitizeHTML(html);
        });

        if (renderVersion === markdownRenderVersion) {
            renderedMarkdownTokens.value = tokenHtmlList;
        }
    } catch (error) {
        console.error('Markdown rendering error:', error);
        if (renderVersion === markdownRenderVersion) {
            renderedMarkdownTokens.value = [];
        }
    }
};

watch(
    () => props.session?.is_completed,
    (completed) => {
        if (completed && props.isLatest) {
            fetchFollowUp();
        }
    },
    { immediate: true }
);

watch(
    () => props.isLatest,
    (latest) => {
        if (latest && props.session?.is_completed) {
            fetchFollowUp();
        }
    }
);

watch(
    () => props.content || props.session?.content || '',
    () => {
        void renderMarkdownTokens();
    },
    { immediate: true }
);

const handleFeedback = async (value) => {
    if (localFeedback.value === value) return; // 已反馈，忽略重复点击
    const msgId = props.session?.id;
    const sessId = typeof props.sessionId === 'object' ? props.sessionId?.value : props.sessionId;
    if (!msgId || !sessId) return;

    const prevFeedback = localFeedback.value;
    localFeedback.value = value;
    try {
        await submitMessageFeedback(sessId, msgId, value);
        MessagePlugin.success(t('chat.feedbackSuccess'));
    } catch (e) {
        localFeedback.value = prevFeedback;
        console.error('反馈提交失败', e);
    }
};

const preview = (url) => {
    nextTick(() => {
        reviewUrl.value = url;
        reviewImg.value = true
    })
}

const closePreImg = () => {
    reviewImg.value = false
    reviewUrl.value = '';
}

// 鍒涘缓鑷畾涔夋覆鏌撳櫒瀹炰緥
// 瑕嗙洊鍥剧墖娓叉煋鏂规硶

// 瑕嗙洊浠ｇ爜鍧楁覆鏌撴柟娉曪紝鏀寔 Mermaid

// 璁＄畻灞炴€э細灏?Markdown 鏂囨湰杞崲涓?tokens
const mentionedItems = computed(() => {
    return props.session?.mentioned_items || [];
});

// 璁＄畻灞炴€э細鍒ゆ柇鏄惁鏈夊疄闄呭唴瀹癸紙闈炵┖涓斾笉鍙槸绌虹櫧锛?
const hasActualContent = computed(() => {
    const text = props.content || props.session?.content || '';
    return text && text.trim().length > 0;
});

// 鑾峰彇瀹為檯鍐呭
const getActualContent = () => {
    return (props.content || props.session?.content || '').trim();
};

// 澶嶅埗鍥炵瓟鍐呭
const handleCopyAnswer = async () => {
    const content = getActualContent();
    if (!content) {
        MessagePlugin.warning(t('chat.emptyContentWarning'));
        return;
    }

    try {
        await copyTextToClipboard(content);
        MessagePlugin.success(t('chat.copySuccess'));
    } catch (err) {
        console.error('复制失败:', err);
        MessagePlugin.error(t('chat.copyFailed'));
    }
};

// 娣诲姞鍒扮煡璇嗗簱
const handleAddToKnowledge = () => {
    const content = getActualContent();
    if (!content) {
        MessagePlugin.warning(t('chat.emptyContentWarning'));
        return;
    }

    const question = (props.userQuery || '').trim();
    const manualContent = buildManualMarkdown(question, content);
    const manualTitle = formatManualTitle(question);
    uiStore.openManualEditor({
        mode: 'create',
        title: manualTitle,
        content: manualContent,
        status: 'draft',
    });

    MessagePlugin.info(t('chat.editorOpened'));
};

// 澶勭悊 markdown-content 涓浘鐗囩殑鐐瑰嚮浜嬩欢
const handleMarkdownImageClick = (e) => {
    const target = e.target;
    if (target && target.tagName === 'IMG') {
        const src = target.getAttribute('src');
        if (src) {
            e.preventDefault();
            e.stopPropagation();
            preview(src);
        }
    }
};

// 娓叉煋 Mermaid 鍥捐〃鐨勫嚱鏁?
const renderMermaidDiagrams = async () => {
  await renderMermaidInContainer(parentMd.value);
};

// 鐩戝惉鍐呭鍙樺寲骞舵覆鏌?Mermaid - 鍙湪浼氳瘽瀹屾垚鍚庢覆鏌?
onUpdated(() => {
    nextTick(async () => {
        await hydrateProtectedFileImages(parentMd.value);
        // 鍙湪浼氳瘽瀹屾垚鍚庢覆鏌?mermaid
        if (props.session?.is_completed) {
            renderMermaidDiagrams();
        }
    });
});

onMounted(async () => {
    // 涓?markdown-content 涓殑鍥剧墖娣诲姞鐐瑰嚮浜嬩欢
    nextTick(async () => {
        if (parentMd.value) {
            parentMd.value.addEventListener('click', handleMarkdownImageClick, true);
        }
        await hydrateProtectedFileImages(parentMd.value);
        // 鍒濆娓叉煋 Mermaid 鍥捐〃
        renderMermaidDiagrams();
    });
});

onBeforeUnmount(() => {
    if (parentMd.value) {
        parentMd.value.removeEventListener('click', handleMarkdownImageClick, true);
    }
});
</script>
<style lang="less" scoped>
@import '../../../components/css/markdown.less';
@import '../../../components/css/chat-message-shared.less';

// 鍐呭鍖呰鍣?- 涓?Agent 妯″紡鐨?answer 鏍峰紡涓€鑷?
.content-wrapper {
    background: var(--td-bg-color-container);
    border-radius: 6px;
    padding: 8px 0px;
    transition: all 0.2s ease;
}

.mentioned_items {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    justify-content: flex-start;
    max-width: 100%;
    margin-bottom: 2px;
}

.mentioned_tag {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    border-radius: 4px;
    font-size: 12px;
    font-weight: 500;
    max-width: 200px;
    cursor: default;
    transition: all 0.15s;
    background: rgba(7, 192, 95, 0.06);
    border: 1px solid rgba(7, 192, 95, 0.2);
    color: var(--td-text-color-primary);

    &.kb-tag {
        .tag_icon {
            color: var(--td-brand-color);
        }
    }

    &.faq-tag {
        .tag_icon {
            color: var(--td-warning-color);
        }
    }

    &.file-tag {
        .tag_icon {
            color: var(--td-text-color-secondary);
        }
    }

    .tag_icon {
        font-size: 13px;
        display: flex;
        align-items: center;
    }

    .tag_name {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: currentColor;
    }
}

.fallback-icon-btn {
    color: var(--td-text-color-disabled) !important;
    border-color: var(--td-component-stroke) !important;

    &:hover {
        color: var(--td-text-color-placeholder) !important;
        border-color: var(--td-component-border) !important;
    }
}

.feedback-btn {
    transition: all 0.15s ease;
}

.feedback-active-like {
    color: var(--td-success-color) !important;
    border-color: var(--td-success-color) !important;
    background: rgba(0, 168, 112, 0.06) !important;
}

.feedback-active-dislike {
    color: var(--td-error-color) !important;
    border-color: var(--td-error-color) !important;
    background: rgba(229, 75, 75, 0.06) !important;
}

@keyframes fadeInUp {
    from {
        opacity: 0;
        transform: translateY(8px);
    }
    to {
        opacity: 1;
        transform: translateY(0);
    }
}

.ai-markdown-template {
    font-size: 15px;
    color: var(--td-text-color-primary);
    line-height: 1.6;
}

.markdown-content {
    :deep(p) {
        margin: 6px 0;
        line-height: 1.6;
    }

    :deep(code) {
        background: var(--td-bg-color-secondarycontainer);
        padding: 2px 5px;
        border-radius: 3px;
        font-family: 'Monaco', 'Menlo', 'Courier New', monospace;
        font-size: 11px;
    }

    :deep(pre) {
        background: var(--td-bg-color-secondarycontainer);
        padding: 10px;
        border-radius: 4px;
        overflow-x: auto;
        margin: 6px 0;

        code {
            background: none;
            padding: 0;
        }
    }

    :deep(ul), :deep(ol) {
        margin: 6px 0;
        padding-left: 20px;
    }

    :deep(li) {
        margin: 3px 0;
    }

    :deep(blockquote) {
        border-left: 2px solid var(--td-brand-color);
        padding-left: 10px;
        margin: 6px 0;
        color: var(--td-text-color-secondary);
    }

    :deep(h1), :deep(h2), :deep(h3), :deep(h4), :deep(h5), :deep(h6) {
        margin: 10px 0 6px 0;
        font-weight: 600;
        color: var(--td-text-color-primary);
    }

    :deep(a) {
        color: var(--td-brand-color);
        text-decoration: none;

        &:hover {
            text-decoration: underline;
        }
    }

    :deep(table) {
        border-collapse: collapse;
        margin: 6px 0;
        font-size: 11px;
        width: 100%;

        th, td {
            border: 1px solid var(--td-component-stroke);
            padding: 5px 8px;
            text-align: left;
        }

        th {
            background: var(--td-bg-color-secondarycontainer);
            font-weight: 600;
        }

        tbody tr:nth-child(even) {
            background: var(--td-bg-color-secondarycontainer);
        }
    }

    :deep(img) {
        max-width: 80%;
        max-height: 300px;
        width: auto;
        height: auto;
        border-radius: 8px;
        display: block;
        margin: 8px 0;
        border: 0.5px solid var(--td-component-stroke);
        object-fit: contain;
        cursor: pointer;
        transition: transform 0.2s ease;

        &:hover {
        }
    }

    // Mermaid 鍥捐〃鏍峰紡
    :deep(.mermaid) {
        margin: 16px 0;
        padding: 16px;
        background: var(--td-bg-color-secondarycontainer);
        border-radius: 8px;
        overflow-x: auto;
        text-align: center;

        svg {
            max-width: 100%;
            height: auto;
        }
    }
}

.ai-markdown-img {
    max-width: 80%;
    max-height: 300px;
    width: auto;
    height: auto;
    border-radius: 8px;
    display: block;
    cursor: pointer;
    object-fit: contain;
    margin: 8px 0 8px 16px;
    border: 0.5px solid var(--td-component-stroke);
    transition: transform 0.2s ease;

    &:hover {
        transform: scale(1.02);
    }
}

.bot_msg {
    // background: var(--td-bg-color-container);
    border-radius: 4px;
    color: var(--td-text-color-primary);
    font-size: 16px;
    // padding: 10px 12px;
    margin-right: auto;
    max-width: 100%;
    box-sizing: border-box;
}

.botanswer_laoding_gif {
    width: 24px;
    height: 18px;
    margin-left: 16px;
}

.thinking-loading {
    padding: 8px 0;
}

.loading-indicator {
    padding: 8px 0;
}

.loading-typing {
    display: flex;
    align-items: center;
    gap: 4px;
    
    span {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        background: var(--td-brand-color);
        animation: typingBounce 1.4s ease-in-out infinite;
        
        &:nth-child(1) {
            animation-delay: 0s;
        }
        
        &:nth-child(2) {
            animation-delay: 0.2s;
        }
        
        &:nth-child(3) {
            animation-delay: 0.4s;
        }
    }
}

@keyframes typingBounce {
    0%, 60%, 100% {
        transform: translateY(0);
    }
    30% {
        transform: translateY(-8px);
    }
}

.img_loading {
    background: var(--td-bg-color-container-hover);
    height: 230px;
    width: 230px;
    color: var(--td-text-color-placeholder);
    display: flex;
    align-items: center;
    justify-content: center;
    flex-direction: column;
    font-size: 12px;
    gap: 4px;
    margin-left: 16px;
    border-radius: 8px;
}

:deep(.t-loading__gradient-conic) {
    background: conic-gradient(from 90deg at 50% 50%, #fff 0deg, #676767 360deg) !important;

}

.agent-toolbar {
    margin-top: 4px;
}

.follow-up-section {
    margin-top: 12px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    animation: fadeInUp 0.25s ease;
}

.follow-up-label {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    font-weight: 500;
}

.follow-up-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
}

.follow-up-chip {
    display: inline-flex;
    align-items: center;
    padding: 5px 12px;
    border-radius: 16px;
    font-size: 13px;
    cursor: pointer;
    border: 1px solid var(--td-component-stroke);
    background: var(--td-bg-color-container);
    color: var(--td-text-color-primary);
    transition: all 0.15s ease;
    line-height: 1.4;

    &:hover {
        background: var(--td-brand-color-light);
        border-color: var(--td-brand-color);
        color: var(--td-brand-color);
    }
}
</style>

