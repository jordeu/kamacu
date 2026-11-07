import { useEffect } from "react";
import { Link, useLocation } from "react-router";
import { Inbox } from "lucide-react";
import { useInbox } from "@/api/inbox";
import { KamacuMark } from "@/components/brand/KamacuMark";
import { Button } from "@/components/ui/button";
import { SidebarTrigger, useSidebar } from "@/components/ui/sidebar";

export function MobileNavigation() {
  const location = useLocation();
  const { setOpenMobile } = useSidebar();
  const { data } = useInbox("pending", 0, "oldest");

  const count = data?.pending_count ?? 0;

  useEffect(() => {
    setOpenMobile(false);
  }, [location.key, setOpenMobile]);

  return (
    <nav aria-label="Mobile navigation" className="flex shrink-0 items-center gap-2 border-b bg-sidebar px-3 pt-[env(safe-area-inset-top)] md:hidden">
      <SidebarTrigger aria-label="Open projects and navigation" className="size-11" />
      <Link to="/" aria-label="Kamacu home" className="flex min-h-11 items-center gap-2 text-sm font-medium">
        <KamacuMark className="size-5" />
        Kamacu
      </Link>
      <Button asChild variant="ghost" className="ml-auto min-h-11 gap-2">
        <Link to="/inbox" aria-current={location.pathname === "/inbox" ? "page" : undefined}>
          <Inbox className="size-4" />
          Inbox
          {count > 0 && <span className="rounded-full bg-orange-500/20 px-1.5 text-xs text-orange-400" aria-label={`${count} pending messages`}>{count > 99 ? "99+" : count}</span>}
        </Link>
      </Button>
    </nav>
  );
}
