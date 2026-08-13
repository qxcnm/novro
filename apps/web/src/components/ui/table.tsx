"use client"

import * as React from "react"

import { cn } from "@/lib/utils"

/**
 * Table 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function Table({ className, ...props }: React.ComponentProps<"table">) {
  return (
    <div
      data-slot="table-container"
      className="relative w-full overflow-x-auto"
    >
      <table
        data-slot="table"
        className={cn("w-full caption-bottom text-sm", className)}
        {...props}
      />
    </div>
  )
}

/**
 * TableHeader 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  return (
    <thead
      data-slot="table-header"
      className={cn("[&_tr]:border-b", className)}
      {...props}
    />
  )
}

/**
 * TableBody 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
  return (
    <tbody
      data-slot="table-body"
      className={cn("[&_tr:last-child]:border-0", className)}
      {...props}
    />
  )
}

/**
 * TableFooter 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function TableFooter({ className, ...props }: React.ComponentProps<"tfoot">) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn(
        "border-t bg-muted/50 font-medium [&>tr]:last:border-b-0",
        className
      )}
      {...props}
    />
  )
}

/**
 * TableRow 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        "border-b transition-colors hover:bg-muted/50 has-aria-expanded:bg-muted/50 data-[state=selected]:bg-muted",
        className
      )}
      {...props}
    />
  )
}

/**
 * TableHead 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function TableHead({ className, ...props }: React.ComponentProps<"th">) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        "h-10 px-2 text-left align-middle font-medium whitespace-nowrap text-foreground [&:has([role=checkbox])]:pr-0",
        className
      )}
      {...props}
    />
  )
}

/**
 * TableCell 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function TableCell({ className, ...props }: React.ComponentProps<"td">) {
  return (
    <td
      data-slot="table-cell"
      className={cn(
        "p-2 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0",
        className
      )}
      {...props}
    />
  )
}

/**
 * TableCaption 渲染对应的 React 界面组件。
 * @param className 用于标识或筛选目标的文本值。
 * @param ...props 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function TableCaption({
  className,
  ...props
}: React.ComponentProps<"caption">) {
  return (
    <caption
      data-slot="table-caption"
      className={cn("mt-4 text-sm text-muted-foreground", className)}
      {...props}
    />
  )
}

export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableHead,
  TableRow,
  TableCell,
  TableCaption,
}
