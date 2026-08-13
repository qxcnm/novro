"use client";

import { useCallback, useMemo, useState } from "react";

/**
 * useListSelection 封装该名称对应的业务处理逻辑。
 * @param ids 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function useListSelection(ids: readonly string[]) {
  const [selection, setSelection] = useState<Set<string>>(() => new Set());
  const available = useMemo(() => new Set(ids), [ids]);
  const selectedIds = useMemo(
    () => [...selection].filter((id) => available.has(id)),
    [available, selection],
  );
  const allSelected = ids.length > 0 && selectedIds.length === available.size;

  const toggleOne = useCallback((id: string, checked: boolean) => {
    setSelection((current) => {
      const next = new Set(current);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);

  const toggleAll = useCallback((checked: boolean) => {
    setSelection((current) => {
      const next = new Set(current);
      for (const id of ids) {
        if (checked) next.add(id);
        else next.delete(id);
      }
      return next;
    });
  }, [ids]);

  const clearSelection = useCallback(() => setSelection(new Set()), []);
  const replaceSelection = useCallback((nextIds: readonly string[]) => setSelection(new Set(nextIds)), []);

  return {
    allSelected,
    checkboxState: allSelected ? true : selectedIds.length > 0 ? "indeterminate" as const : false,
    clearSelection,
    isSelected: (id: string) => selection.has(id),
    replaceSelection,
    selectedIds,
    toggleAll,
    toggleOne,
  };
}
