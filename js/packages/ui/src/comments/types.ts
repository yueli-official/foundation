export type PublicCommentOrder = "asc" | "desc";
export type PublicCommentState = "loading" | "ready" | "error";

export interface PublicComment {
  readonly id: string;
  readonly parentId?: string;
  readonly authorName: string;
  readonly avatarUrl?: string;
  readonly isAnonymous?: boolean;
  readonly content: string;
  readonly createdAt: string;
  readonly replies?: readonly PublicComment[];
}

export interface PublicCommentViewer {
  readonly authenticated: boolean;
  readonly name: string;
  readonly avatarUrl?: string;
}

export interface PublicCommentDraft {
  readonly content: string;
  readonly parentId?: string;
  readonly authorName?: string;
  readonly authorEmail?: string;
}

export interface PublicCommentSubmitResult {
  readonly pending: boolean;
}

export type PublicCommentSubmit = (
  draft: PublicCommentDraft,
) => Promise<PublicCommentSubmitResult>;

export interface PublicCommentMessages {
  readonly count: (count: number) => string;
  readonly replies: (count: number) => string;
  readonly sort: string;
  readonly loading: string;
  readonly oldest: string;
  readonly newest: string;
  readonly reply: string;
  readonly cancelReply: string;
  readonly anonymous: string;
  readonly empty: string;
  readonly closed: string;
  readonly loadError: string;
  readonly retry: string;
  readonly writeComment: string;
  readonly writeReply: string;
  readonly authorName: string;
  readonly authorEmail: string;
  readonly anonymousHint: string;
  readonly login: string;
  readonly submit: string;
  readonly submitReply: string;
  readonly submitted: string;
  readonly pending: string;
  readonly submitError: string;
  readonly nameRequired: string;
}
