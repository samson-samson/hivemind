import type { HivemindApi } from './client';
import { mockApi } from './mock/client';
import { HttpHivemindApi } from './http/client';

/**
 * 适配器选择：
 *   VITE_API_MODE=mock（默认）→ 内置演示数据，无需后端
 *   VITE_API_MODE=http  → 连接真后端 control-plane（VITE_API_BASE 覆盖地址）
 */
const mode = import.meta.env.VITE_API_MODE ?? 'mock';
export const api: HivemindApi = mode === 'http' ? new HttpHivemindApi() : mockApi;

export * from './types';
export type { HivemindApi } from './client';
