export type CurrentUser = {
  id: string;
  username: string | null;
  displayName: string | null;
  avatarUrl: string | null;
  locale: string;
  roles: string[];
  profileComplete: boolean;
};
export type ApiError = { error: { code: string; message: string; requestId: string } };
