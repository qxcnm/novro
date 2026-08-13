import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

/**
 * cn 封装该名称对应的业务处理逻辑。
 * @param inputs 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
