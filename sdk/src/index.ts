import { getCompatModule } from "./core";

export * from "./core";

export const Button = getCompatModule("@/components/ui/button").Button;
export const Card = getCompatModule("@/components/ui/card").Card;
export const Input = getCompatModule("@/components/ui/input").Input;
export const Switch = getCompatModule("@/components/ui/switch").Switch;
const tableModule = getCompatModule("@/components/ui/table");
export const Table = tableModule.Table;
export const TableBody = tableModule.TableBody;
export const TableCell = tableModule.TableCell;
export const TableFooter = tableModule.TableFooter;
export const TableHead = tableModule.TableHead;
export const TableHeader = tableModule.TableHeader;
export const TableRow = tableModule.TableRow;
export const TableCaption = tableModule.TableCaption;
export const FormDate = getCompatModule("@/page/nodes/form/date").FormDate;
const dialogModule = getCompatModule("@/components/ui/dialog");
export const Dialog = dialogModule.Dialog;
export const DialogContent = dialogModule.DialogContent;
export const DialogDescription = dialogModule.DialogDescription;
export const DialogFooter = dialogModule.DialogFooter;
export const DialogHeader = dialogModule.DialogHeader;
export const DialogTitle = dialogModule.DialogTitle;
export const SiteLogo = getCompatModule(
  "@/components/layout/site-logo",
).SiteLogo;

export const getSiteConfig = getCompatModule(
  "@/config/app-config",
).getSiteConfig;
export const resolvePostLoginTarget = getCompatModule(
  "@/lib/auth-redirect",
).resolvePostLoginTarget;
const requestModule = getCompatModule("@/lib/request");
export const joinFrontApi = requestModule.joinFrontApi;
export const joinSiteApi = requestModule.joinSiteApi;
export const buildRuntimeRequestHeaders =
  requestModule.buildRuntimeRequestHeaders;
export const loadMainInfo = requestModule.loadMainInfo;
export const request = requestModule.request;
export const requestRaw = requestModule.requestRaw;
export const resetFrontRuntimeCache =
  requestModule.resetFrontRuntimeCache;
export const useAuthStore = getCompatModule("@/stores/auth-store").useAuthStore;
export const useTheme = getCompatModule("@/context/theme-provider").useTheme;
