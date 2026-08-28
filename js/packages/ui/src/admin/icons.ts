import type { AdminIconOption } from "./types";

/**
 * Finite, offline-safe Tabler catalog for administration surfaces. Every icon
 * is a literal so the Nuxt Icon scanner includes its SVG in the client bundle.
 * Products can append domain icons through AdminIconPicker.options and
 * yueliUi.tablerIcons without shipping the complete 2 MB Tabler collection.
 */
export const ADMIN_TABLER_ICON_OPTIONS: readonly AdminIconOption[] = [
  { label: "应用", value: "i-tabler-apps", keywords: ["app", "application"] },
  { label: "网站", value: "i-tabler-world-www", keywords: ["web", "site"] },
  { label: "首页", value: "i-tabler-home", keywords: ["home"] },
  { label: "控制台", value: "i-tabler-dashboard", keywords: ["dashboard"] },
  { label: "布局", value: "i-tabler-layout-dashboard", keywords: ["layout"] },
  { label: "文档集", value: "i-tabler-stack-2", keywords: ["stack", "collection"] },
  { label: "文档", value: "i-tabler-book-2", keywords: ["book", "docs"] },
  { label: "正文", value: "i-tabler-file-text", keywords: ["file", "text"] },
  { label: "文件", value: "i-tabler-files", keywords: ["files"] },
  { label: "说明", value: "i-tabler-file-description", keywords: ["description"] },
  { label: "笔记", value: "i-tabler-notes", keywords: ["notes"] },
  { label: "手册", value: "i-tabler-manual-gearbox", keywords: ["manual"] },
  { label: "代码", value: "i-tabler-code", keywords: ["code"] },
  { label: "代码块", value: "i-tabler-code-dots", keywords: ["code", "snippet"] },
  { label: "终端", value: "i-tabler-terminal-2", keywords: ["terminal", "cli"] },
  { label: "接口", value: "i-tabler-api", keywords: ["api"] },
  { label: "组件", value: "i-tabler-components", keywords: ["component"] },
  { label: "产品", value: "i-tabler-package", keywords: ["package", "product"] },
  { label: "工具", value: "i-tabler-tool", keywords: ["tool"] },
  { label: "设置", value: "i-tabler-settings", keywords: ["setting", "config"] },
  { label: "调整", value: "i-tabler-adjustments-horizontal", keywords: ["adjust", "filter"] },
  { label: "数据", value: "i-tabler-database", keywords: ["database", "data"] },
  { label: "数据设置", value: "i-tabler-database-cog", keywords: ["database", "storage"] },
  { label: "服务器", value: "i-tabler-server", keywords: ["server"] },
  { label: "云端", value: "i-tabler-cloud", keywords: ["cloud"] },
  { label: "上传", value: "i-tabler-cloud-upload", keywords: ["upload", "cloud"] },
  { label: "快速开始", value: "i-tabler-rocket", keywords: ["rocket", "start"] },
  { label: "发布", value: "i-tabler-world-upload", keywords: ["publish", "deploy"] },
  { label: "链接", value: "i-tabler-link", keywords: ["link", "url"] },
  { label: "外部链接", value: "i-tabler-external-link", keywords: ["external", "link"] },
  { label: "目录", value: "i-tabler-sitemap", keywords: ["sitemap", "toc"] },
  { label: "层级", value: "i-tabler-hierarchy-2", keywords: ["hierarchy", "tree"] },
  { label: "文件夹", value: "i-tabler-folder", keywords: ["folder"] },
  { label: "文件夹组", value: "i-tabler-folders", keywords: ["folders"] },
  { label: "分类", value: "i-tabler-category", keywords: ["category"] },
  { label: "标签", value: "i-tabler-tags", keywords: ["tag"] },
  { label: "图片", value: "i-tabler-photo", keywords: ["photo", "image"] },
  { label: "添加图片", value: "i-tabler-photo-plus", keywords: ["photo", "image", "add"] },
  { label: "配色", value: "i-tabler-palette", keywords: ["palette", "color"] },
  { label: "星标", value: "i-tabler-star", keywords: ["star", "favorite"] },
  { label: "钻石", value: "i-tabler-diamond", keywords: ["diamond"] },
  { label: "喜欢", value: "i-tabler-heart", keywords: ["heart", "like"] },
  { label: "收藏", value: "i-tabler-bookmark", keywords: ["bookmark"] },
  { label: "评论", value: "i-tabler-message", keywords: ["message", "comment"] },
  { label: "消息", value: "i-tabler-messages", keywords: ["messages", "chat"] },
  { label: "用户", value: "i-tabler-user", keywords: ["user"] },
  { label: "用户组", value: "i-tabler-users", keywords: ["users", "team"] },
  { label: "用户权限", value: "i-tabler-user-shield", keywords: ["user", "permission"] },
  { label: "权限", value: "i-tabler-shield-lock", keywords: ["shield", "permission"] },
  { label: "审核", value: "i-tabler-shield-check", keywords: ["shield", "review"] },
  { label: "安全", value: "i-tabler-lock-check", keywords: ["security", "lock"] },
  { label: "密钥", value: "i-tabler-key", keywords: ["key", "credential"] },
  { label: "通知", value: "i-tabler-bell", keywords: ["bell", "notification"] },
  { label: "邮件", value: "i-tabler-mail", keywords: ["mail", "email"] },
  { label: "日历", value: "i-tabler-calendar", keywords: ["calendar", "date"] },
  { label: "时间", value: "i-tabler-clock", keywords: ["clock", "time"] },
  { label: "柱状图", value: "i-tabler-chart-bar", keywords: ["chart", "analytics"] },
  { label: "趋势", value: "i-tabler-chart-line", keywords: ["chart", "trend"] },
  { label: "搜索", value: "i-tabler-search", keywords: ["search"] },
  { label: "GitHub", value: "i-tabler-brand-github", keywords: ["github", "git"] },
  { label: "哔哩哔哩", value: "i-tabler-brand-bilibili", keywords: ["bilibili", "b站"] },
  { label: "实验", value: "i-tabler-flask", keywords: ["flask", "experiment"] },
];

export function filterAdminIconOptions(
  options: readonly AdminIconOption[],
  query: string,
): readonly AdminIconOption[] {
  const normalized = query.trim().toLocaleLowerCase();
  if (!normalized) return options;
  return options.filter((option) =>
    [option.label, option.value.replace(/^i-tabler-/, ""), ...(option.keywords || [])]
      .join(" ")
      .toLocaleLowerCase()
      .includes(normalized),
  );
}
