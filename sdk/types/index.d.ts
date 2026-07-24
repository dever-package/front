import type { ComponentType, LazyExoticComponent } from "react";

export type NodeItemProps = {
  item: any;
  store?: any;
};

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
  nodes?: NodeComponentRegistry;
};

export function defineFrontPlugin(plugin: DeverFrontPlugin): DeverFrontPlugin;
export function lazyNode(loader: LazyNodeLoader): LazyNodeComponent;
export function mergePluginNodes(
  plugins: DeverFrontPlugin[],
): NodeComponentRegistry;
export function getCompatModule(path: string): Record<string, any>;

export function useNavigate(...args: any[]): any;
export function useSearch(...args: any[]): any;

export const Button: any;
export const Card: any;
export const Input: any;
export const Switch: any;
export const Table: any;
export const TableBody: any;
export const TableCell: any;
export const TableFooter: any;
export const TableHead: any;
export const TableHeader: any;
export const TableRow: any;
export const TableCaption: any;
export const FormDate: any;
export const Dialog: any;
export const DialogContent: any;
export const DialogDescription: any;
export const DialogFooter: any;
export const DialogHeader: any;
export const DialogTitle: any;
export const SiteLogo: any;

export const getSiteConfig: any;
export const resolvePostLoginTarget: any;
export const joinFrontApi: any;
export const joinSiteApi: any;
export const buildRuntimeRequestHeaders: any;
export const loadMainInfo: any;
export const request: any;
export const requestRaw: any;
export const resetFrontRuntimeCache: any;
export const useAuthStore: any;
export const useTheme: () => {
  defaultTheme: "dark" | "light" | "system";
  resolvedTheme: "dark" | "light";
  theme: "dark" | "light" | "system";
  setTheme: (theme: "dark" | "light" | "system") => void;
  resetTheme: () => void;
};
