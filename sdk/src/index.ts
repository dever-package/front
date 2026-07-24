import { lazy, type ComponentType, type LazyExoticComponent } from "react";

export type LazyNodeLoader = () => Promise<{
  default: ComponentType<NodeItemProps>;
}>;

export type LazyNodeComponent = LazyExoticComponent<
  ComponentType<NodeItemProps>
> & {
  preload: () => Promise<unknown>;
};

export type NodeComponentRegistry = Record<string, LazyNodeComponent>;

export type DeverFrontPlugin = {
  name: string;
  depends?: string[];
  nodes?: NodeComponentRegistry;
};

export type NodeItemProps = {
  item: any;
  store?: any;
};

type DeverFrontSDK = {
  defineFrontPlugin: (plugin: DeverFrontPlugin) => DeverFrontPlugin;
  lazyNode: (loader: LazyNodeLoader) => LazyNodeComponent;
  mergePluginNodes: (plugins: DeverFrontPlugin[]) => NodeComponentRegistry;
  useNavigate: (...args: any[]) => any;
  useSearch: (...args: any[]) => any;
  getCompatModule: (path: string) => Record<string, any>;
};

declare global {
  interface Window {
    DeverFront?: {
      registerPlugin?: (plugin: DeverFrontPlugin) => void;
      sdk?: DeverFrontSDK;
    };
  }
}

export function defineFrontPlugin(plugin: DeverFrontPlugin) {
  return window.DeverFront?.sdk?.defineFrontPlugin?.(plugin) || plugin;
}

export function lazyNode(loader: LazyNodeLoader): LazyNodeComponent {
  const sdk = window.DeverFront?.sdk;
  if (sdk?.lazyNode) {
    return sdk.lazyNode(loader);
  }

  let preloadPromise: ReturnType<LazyNodeLoader> | null = null;
  const load = () => {
    if (!preloadPromise) {
      preloadPromise = loader().catch((error) => {
        preloadPromise = null;
        throw error;
      });
    }
    return preloadPromise;
  };
  const component = lazy(load) as LazyNodeComponent;
  component.preload = load;
  return component;
}

export function mergePluginNodes(plugins: DeverFrontPlugin[]) {
  const merge = window.DeverFront?.sdk?.mergePluginNodes;
  if (merge) {
    return merge(plugins);
  }

  const nodes: NodeComponentRegistry = {};
  for (const plugin of plugins) {
    Object.assign(nodes, plugin.nodes);
  }
  return nodes;
}

export function getCompatModule(path: string) {
  return frontSDK().getCompatModule(path);
}

export function useNavigate(...args: any[]) {
  return frontSDK().useNavigate(...args);
}

export function useSearch(...args: any[]) {
  return frontSDK().useSearch(...args);
}

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
export const joinFrontApi = getCompatModule("@/lib/request").joinFrontApi;
export const joinSiteApi = getCompatModule("@/lib/request").joinSiteApi;
export const buildRuntimeRequestHeaders =
  getCompatModule("@/lib/request").buildRuntimeRequestHeaders;
export const loadMainInfo = getCompatModule("@/lib/request").loadMainInfo;
export const request = getCompatModule("@/lib/request").request;
export const requestRaw = getCompatModule("@/lib/request").requestRaw;
export const resetFrontRuntimeCache =
  getCompatModule("@/lib/request").resetFrontRuntimeCache;
export const useAuthStore = getCompatModule("@/stores/auth-store").useAuthStore;
export const useTheme = getCompatModule(
  "@/context/theme-provider",
).useTheme;

function frontSDK(): DeverFrontSDK {
  const sdk = window.DeverFront?.sdk;
  if (!sdk) {
    throw new Error("Dever front plugin SDK is not ready");
  }
  return sdk;
}
