import { useRouter } from 'vue-router'

/**
 * Provides a shared navigation helper for knowledge-base creation success.
 * Redirects to the new knowledge-base detail page so the user can continue with import.
 */
export const useKnowledgeBaseCreationNavigation = () => {
  const router = useRouter()

  const navigateToKnowledgeBaseList = (kbId: string) => {
    if (!kbId) return
    router.push({
      path: `/platform/knowledge-bases/${kbId}`,
      query: { action: 'upload', created: '1' },
    })
  }

  return {
    navigateToKnowledgeBaseList,
  }
}

