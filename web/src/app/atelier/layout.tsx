import type { ReactNode } from "react";
import { Workspace } from "../../components/Shell";
import { adminApi } from "../../lib/api";
import { publicConfig } from "../../lib/public-site";
export const metadata = {
  title: "Atelier enseignant",
  robots: { index: false, follow: false },
};
export default async function Layout({ children }: { children: ReactNode }) {
  const [{ user }, { salesEnabled }] = await Promise.all([
    adminApi(),
    publicConfig(),
  ]);
  return (
    <Workspace salesEnabled={salesEnabled} user={user} teacher>
      {children}
    </Workspace>
  );
}
