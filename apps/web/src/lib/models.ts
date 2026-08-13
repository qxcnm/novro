export type ModelVendor = "zhipu" | "deepseek" | "moonshot";

export type ModelPriceTier = {
  label?: string;
  cachedInput: number;
  input: number;
  output: number;
};

export type ModelEntry = {
  id: string;
  name: string;
  vendor: ModelVendor;
  vendorName: string;
  description: string;
  contextTokens: number;
  contextLabel: string;
  maxOutputLabel: string;
  inputTypes: Array<"文本" | "图片" | "视频">;
  capabilities: string[];
  pricing: ModelPriceTier[] | null;
  officialModel: boolean;
  officialSource: string;
  officialSourceLabel: string;
  releasedAt: string | null;
  featured?: boolean;
};

export const PRICE_VERIFIED_AT = "2026-08-05";

export const modelCatalog: ModelEntry[] = [
  {
    id: "glm-5.2",
    name: "GLM-5.2",
    vendor: "zhipu",
    vendorName: "智谱 GLM",
    description: "面向长任务的旗舰基座模型，强调项目级工程上下文、长程 Coding 与稳定工具调用。",
    contextTokens: 1_000_000,
    contextLabel: "1M",
    maxOutputLabel: "128K",
    inputTypes: ["文本"],
    capabilities: ["深度思考", "工具调用", "结构化输出", "上下文缓存"],
    pricing: [{ cachedInput: 2, input: 8, output: 28 }],
    officialModel: true,
    officialSource: "https://open.bigmodel.cn/pricing",
    officialSourceLabel: "智谱官方价格",
    releasedAt: "2026-06-16",
    featured: true,
  },
  {
    id: "glm-5.2-sale",
    name: "GLM-5.2 Sale",
    vendor: "zhipu",
    vendorName: "智谱 GLM",
    description: "Novro 预留的优惠路由别名，不是智谱公开价目中的独立官方模型，实际映射与售价待上线后公布。",
    contextTokens: 1_000_000,
    contextLabel: "1M 规划",
    maxOutputLabel: "128K 规划",
    inputTypes: ["文本"],
    capabilities: ["规划路由", "售价待定"],
    pricing: null,
    officialModel: false,
    officialSource: "https://open.bigmodel.cn/pricing",
    officialSourceLabel: "对照智谱公开价目",
    releasedAt: null,
  },
  {
    id: "glm-5.2-fast",
    name: "GLM-5.2 Fast",
    vendor: "zhipu",
    vendorName: "智谱 GLM",
    description: "Novro 预留的高速路由别名，不是智谱公开价目中的独立官方模型，速度指标与售价待上线验证。",
    contextTokens: 1_000_000,
    contextLabel: "1M 规划",
    maxOutputLabel: "128K 规划",
    inputTypes: ["文本"],
    capabilities: ["高速规划", "售价待定"],
    pricing: null,
    officialModel: false,
    officialSource: "https://open.bigmodel.cn/pricing",
    officialSourceLabel: "对照智谱公开价目",
    releasedAt: null,
  },
  {
    id: "glm-5.1",
    name: "GLM-5.1",
    vendor: "zhipu",
    vendorName: "智谱 GLM",
    description: "面向 Agentic Coding、复杂指令与长文档生产的旗舰模型，支持长程规划和工程级交付。",
    contextTokens: 200_000,
    contextLabel: "200K",
    maxOutputLabel: "128K",
    inputTypes: ["文本"],
    capabilities: ["深度思考", "工具调用", "结构化输出", "MCP"],
    pricing: [
      { label: "输入长度 0-32K", cachedInput: 1.3, input: 6, output: 24 },
      { label: "输入长度 32K+", cachedInput: 2, input: 8, output: 28 },
    ],
    officialModel: true,
    officialSource: "https://open.bigmodel.cn/pricing",
    officialSourceLabel: "智谱官方价格",
    releasedAt: "2026-04-07",
  },
  {
    id: "deepseek-v4-pro",
    name: "DeepSeek-V4 Pro",
    vendor: "deepseek",
    vendorName: "DeepSeek",
    description: "V4 的高性能版本，面向复杂推理、数学、代码与科研分析，支持思考和非思考模式。",
    contextTokens: 1_000_000,
    contextLabel: "1M",
    maxOutputLabel: "384K",
    inputTypes: ["文本"],
    capabilities: ["深度思考", "Tool Calls", "JSON Output", "Anthropic API"],
    pricing: [{ cachedInput: 0.025, input: 3, output: 6 }],
    officialModel: true,
    officialSource: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/",
    officialSourceLabel: "DeepSeek 官方价格",
    releasedAt: "2026-04-24",
    featured: true,
  },
  {
    id: "deepseek-v4-flash",
    name: "DeepSeek-V4 Flash",
    vendor: "deepseek",
    vendorName: "DeepSeek",
    description: "V4 的高吞吐版本，面向在线服务与批量任务，同时支持 Responses API 和 Anthropic API。",
    contextTokens: 1_000_000,
    contextLabel: "1M",
    maxOutputLabel: "384K",
    inputTypes: ["文本"],
    capabilities: ["深度思考", "Tool Calls", "Responses API", "JSON Output"],
    pricing: [{ cachedInput: 0.02, input: 1, output: 2 }],
    officialModel: true,
    officialSource: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/",
    officialSourceLabel: "DeepSeek 官方价格",
    releasedAt: "2026-07-31",
  },
  {
    id: "kimi-k3",
    name: "Kimi K3",
    vendor: "moonshot",
    vendorName: "Moonshot Kimi",
    description: "面向长程 Coding 与端到端知识工作的旗舰模型，原生视觉理解并支持可调推理强度。",
    contextTokens: 1_048_576,
    contextLabel: "1M",
    maxOutputLabel: "官方未单列",
    inputTypes: ["文本", "图片", "视频"],
    capabilities: ["长程 Coding", "ToolCalls", "JSON Schema", "动态工具"],
    pricing: [{ cachedInput: 2, input: 20, output: 100 }],
    officialModel: true,
    officialSource: "https://platform.kimi.com/docs/pricing/chat-k3",
    officialSourceLabel: "Kimi 官方价格",
    releasedAt: "2026-07-16",
    featured: true,
  },
  {
    id: "kimi-k2.7-code",
    name: "Kimi K2.7 Code",
    vendor: "moonshot",
    vendorName: "Moonshot Kimi",
    description: "为代码场景优化的 Coding 模型，强化长上下文指令遵循、编程成功率与多模态输入。",
    contextTokens: 262_144,
    contextLabel: "256K",
    maxOutputLabel: "官方未单列",
    inputTypes: ["文本", "图片", "视频"],
    capabilities: ["Coding", "长思考", "ToolCalls", "自动缓存"],
    pricing: [{ cachedInput: 1.3, input: 6.5, output: 27 }],
    officialModel: true,
    officialSource: "https://platform.kimi.com/docs/pricing/chat-k27-code",
    officialSourceLabel: "Kimi 官方价格",
    releasedAt: "2026-06-12",
  },
  {
    id: "kimi-k2.6",
    name: "Kimi K2.6",
    vendor: "moonshot",
    vendorName: "Moonshot Kimi",
    description: "通用多模态模型，支持思考与非思考模式，覆盖对话、代码和 Agent 任务。",
    contextTokens: 262_144,
    contextLabel: "256K",
    maxOutputLabel: "官方未单列",
    inputTypes: ["文本", "图片", "视频"],
    capabilities: ["多模态", "深度推理", "ToolCalls", "JSON Mode"],
    pricing: [{ cachedInput: 1.1, input: 6.5, output: 27 }],
    officialModel: true,
    officialSource: "https://platform.kimi.com/docs/pricing/chat-k26",
    officialSourceLabel: "Kimi 官方价格",
    releasedAt: "2026-04-20",
  },
];

/**
 * compareModelRelease 封装该名称对应的业务处理逻辑。
 * @param left 本次操作需要使用的输入参数。
 * @param right 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function compareModelRelease(left: ModelEntry, right: ModelEntry) {
  if (left.releasedAt === right.releasedAt) {
    return 0;
  }
  if (left.releasedAt === null) {
    return 1;
  }
  if (right.releasedAt === null) {
    return -1;
  }
  return right.releasedAt.localeCompare(left.releasedAt);
}

/**
 * formatPrice 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function formatPrice(value: number) {
  return Number.isInteger(value) ? String(value) : String(value);
}

/**
 * getStartingPrice 封装该名称对应的业务处理逻辑。
 * @param model 本次操作需要使用的输入参数。
 * @param field 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function getStartingPrice(model: ModelEntry, field: keyof Omit<ModelPriceTier, "label">) {
  if (!model.pricing?.length) {
    return null;
  }

  return Math.min(...model.pricing.map((tier) => tier[field]));
}
