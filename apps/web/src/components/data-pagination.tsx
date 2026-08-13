"use client";

import { type FormEvent, useId } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

const pageSizes = [20, 50, 100];

type DataPaginationProps = {
  loading: boolean;
  offset: number;
  onOffsetChange: (offset: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  pageSize: number;
  total: number;
};

/**
 * DataPagination 渲染对应的 React 界面组件。
 * @param loading 本次操作需要使用的输入参数。
 * @param offset 本次操作使用的数值参数。
 * @param onOffsetChange 本次操作需要使用的输入参数。
 * @param onPageSizeChange 本次操作需要使用的输入参数。
 * @param pageSize 本次操作使用的数值参数。
 * @param total 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function DataPagination({ loading, offset, onOffsetChange, onPageSizeChange, pageSize, total }: DataPaginationProps) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(pages, Math.floor(offset / pageSize) + 1);
  const jumpInputID = useId();

  /**
   * goToPage 封装该名称对应的业务处理逻辑。
   * @param page 本次操作需要使用的输入参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  function goToPage(page: number) {
    const nextPage = Math.min(pages, Math.max(1, page));
    onOffsetChange((nextPage - 1) * pageSize);
  }

  /**
   * submitJump 封装该名称对应的业务处理逻辑。
   * @param event 触发当前处理流程的事件。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  function submitJump(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const requested = Number.parseInt(String(new FormData(event.currentTarget).get("page") ?? ""), 10);
    goToPage(Number.isFinite(requested) ? requested : currentPage);
  }

  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t px-4 py-3 text-sm text-muted-foreground">
      <span className="whitespace-nowrap">共 {total.toLocaleString("zh-CN")} 条</span>
      <div className="flex items-center gap-2 whitespace-nowrap">
        <span>每页显示</span>
        <Select
          disabled={loading}
          onValueChange={(value) => onPageSizeChange(Number(value))}
          value={String(pageSize)}
        >
          <SelectTrigger aria-label="每页显示条数" className="w-20">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {pageSizes.map((size) => <SelectItem key={size} value={String(size)}>{size}</SelectItem>)}
          </SelectContent>
        </Select>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button disabled={currentPage === 1 || loading} onClick={() => goToPage(1)} type="button" variant="outline">首页</Button>
        <Button disabled={currentPage === 1 || loading} onClick={() => goToPage(currentPage - 1)} type="button" variant="outline">上一页</Button>
        <span className="min-w-24 text-center font-medium text-foreground">第 {currentPage} / {pages} 页</span>
        <Button disabled={currentPage === pages || loading} onClick={() => goToPage(currentPage + 1)} type="button" variant="outline">下一页</Button>
        <Button disabled={currentPage === pages || loading} onClick={() => goToPage(pages)} type="button" variant="outline">末页</Button>
      </div>
      <form className="ml-auto flex items-center gap-2 whitespace-nowrap" onSubmit={submitJump}>
        <label htmlFor={jumpInputID}>跳至</label>
        <Input
          aria-label="跳转页码"
          className="w-20"
          defaultValue={currentPage}
          disabled={loading}
          id={jumpInputID}
          inputMode="numeric"
          key={`${currentPage}-${pages}-${pageSize}`}
          max={pages}
          min={1}
          name="page"
          type="number"
        />
        <span>页</span>
        <Button disabled={loading} type="submit" variant="outline">确定</Button>
      </form>
    </div>
  );
}
