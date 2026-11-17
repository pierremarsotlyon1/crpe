import type { ReactNode } from "react";
import { Workspace } from "../../components/Shell";
import { memberAccess, memberApi } from "../../lib/api";

export const metadata = {
  title: "Espace élève",
  robots: { index: false, follow: false },
};
export default async function Layout({ children }: { children: ReactNode }) {
  const [{ user }, access] = await Promise.all([memberApi(), memberAccess()]);
  return (
    <Workspace access={access} salesEnabled={access.salesEnabled} user={user}>
      {children}
    </Workspace>
  );
}
