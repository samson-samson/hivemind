import type { OpsHiveApi } from './client';
import { mockApi } from './mock/client';

/**
 * 适配器选择：后端就绪后实现 OpenAPI client（同一 OpsHiveApi 接口），
 * 按 import.meta.env.VITE_API_MODE 切换，此处一行替换即可。
 */
export const api: OpsHiveApi = mockApi;

export * from './types';
export type { OpsHiveApi } from './client';
