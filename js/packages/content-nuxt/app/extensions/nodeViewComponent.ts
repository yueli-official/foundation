import type { NodeViewProps } from "@tiptap/core";
import type { Component } from "vue";

// NodeView 组件只声明自身读取的 props，Tiptap 运行时仍会注入完整 NodeViewProps。
// 这个内部适配点集中表达两者的结构兼容关系，避免各扩展散落类型断言。
export function asNodeViewComponent(
  component: Component,
): Component<NodeViewProps> {
  return component as unknown as Component<NodeViewProps>;
}
