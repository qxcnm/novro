"use client";

import { Boxes, House, Menu, PlugZap, ShieldCheck } from "lucide-react";
import Link from "next/link";

import { ThemeToggle } from "@/components/theme-toggle";
import { Button } from "@/components/ui/button";
import {
  NavigationMenu,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
  navigationMenuTriggerStyle,
} from "@/components/ui/navigation-menu";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";

const navItems = [
  { href: "/", label: "首页", icon: House },
  { href: "/models", label: "模型", icon: Boxes },
  { href: "/docs", label: "接入文档", icon: PlugZap },
];

/**
 * SiteHeader 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function SiteHeader() {
  return (
    <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
      <div className="mx-auto flex h-16 w-full max-w-7xl items-center px-5 sm:px-8 lg:px-10">
        <Link href="/" className="flex items-center gap-3" aria-label="Novro 首页">
          <span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <ShieldCheck className="size-4" aria-hidden="true" />
          </span>
          <span className="text-sm font-semibold">Novro</span>
        </Link>

        <NavigationMenu className="ml-10 hidden md:flex" viewport={false}>
          <NavigationMenuList>
            {navItems.map((item) => (
              <NavigationMenuItem key={item.href}>
                <NavigationMenuLink asChild>
                  <Link href={item.href} className={navigationMenuTriggerStyle()}>
                    {item.label}
                  </Link>
                </NavigationMenuLink>
              </NavigationMenuItem>
            ))}
          </NavigationMenuList>
        </NavigationMenu>

        <div className="ml-auto flex items-center gap-1.5">
          <ThemeToggle />
          <Button asChild className="hidden sm:inline-flex">
            <Link href="/login">进入控制台</Link>
          </Button>
          <Sheet>
            <SheetTrigger asChild>
              <Button className="md:hidden" size="icon" variant="ghost" aria-label="打开导航">
                <Menu aria-hidden="true" />
              </Button>
            </SheetTrigger>
            <SheetContent className="w-[min(22rem,88vw)]">
              <SheetHeader className="border-b">
                <SheetTitle className="flex items-center gap-2">
                  <ShieldCheck className="size-4" aria-hidden="true" />
                  Novro
                </SheetTitle>
                <SheetDescription>统一的大模型 API 接入平台</SheetDescription>
              </SheetHeader>
              <nav className="flex flex-col gap-1 px-3" aria-label="移动端导航">
                {navItems.map((item) => (
                  <SheetClose asChild key={item.href}>
                    <Link
                      href={item.href}
                      className={cn(navigationMenuTriggerStyle(), "w-full justify-start")}
                    >
                      <item.icon aria-hidden="true" />
                      {item.label}
                    </Link>
                  </SheetClose>
                ))}
              </nav>
              <div className="mt-auto border-t p-4">
                <SheetClose asChild>
                  <Button asChild className="w-full">
                    <Link href="/login">进入控制台</Link>
                  </Button>
                </SheetClose>
              </div>
            </SheetContent>
          </Sheet>
        </div>
      </div>
    </header>
  );
}
