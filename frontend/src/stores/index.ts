/**
 * Pinia Stores Export
 * Central export point for all application stores
 */

export { useAuthStore } from './auth'
export { useAppStore } from './app'
export { useAdminSettingsStore } from './adminSettings'

// Re-export types for convenience
export type { User, LoginRequest, AuthResponse } from '@/types'
export type { Toast, ToastType, AppState } from '@/types'
