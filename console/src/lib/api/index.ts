import type { OpsHiveApi } from './client';
import { mockApi } from './mock/client';
import { HttpOpsHiveApi } from './http/client';

/**
 * 适配器选择：
 *   VITE_API_MODE=mock（默认）→ 内置演示数据，无需后端
 *   VITE_API_MODE=http  → 连接真后端 control-plane（VITE_API_BASE 覆盖地址）
 */
const mode = import.meta.env.VITE_API_MODE ?? 'mock';
export const api: OpsHiveApi = mode === 'http' ? new HttpOpsHiveApi() : mockApi;

export * from './types';
export type { OpsHiveApi } from './client';
