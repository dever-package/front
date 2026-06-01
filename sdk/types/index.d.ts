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
export const SiteLogo: any;

export const getSiteConfig: any;
export const resolvePostLoginTarget: any;
export const joinFrontApi: any;
export const joinSiteApi: any;
export const loadMainInfo: any;
export const request: any;
export const resetFrontRuntimeCache: any;
export const useAuthStore: any;
