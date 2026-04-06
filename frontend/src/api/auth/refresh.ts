import { post } from '@/utils/request'
import i18n from '@/i18n'

const t = (key: string) => i18n.global.t(key)

export async function refreshToken(refreshToken: string): Promise<{ success: boolean; data?: { token: string; refreshToken: string }; message?: string }> {
  try {
    const response: any = await post('/api/v1/auth/refresh', { refreshToken })
    if (response && response.success && (response.access_token || response.refresh_token)) {
      return {
        success: true,
        data: {
          token: response.access_token,
          refreshToken: response.refresh_token,
        }
      }
    }

    return {
      success: false,
      message: response?.message || t('error.auth.refreshTokenFailed')
    }
  } catch (error: any) {
    return {
      success: false,
      message: error.message || t('error.auth.refreshTokenFailed')
    }
  }
}
