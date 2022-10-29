import { PluginDefine } from "@certd/pipeline/src";

export type PipelineDetail = {
  pipeline: PipelineDefine;
};

export type RunHistory = {
  id: any;
  pipeline: PipelineDefine;
  logs?: {
    [id: string]: string[];
  };
};
export type RunHistoryLog = {
  logs: {
    [id: string]: string[];
  };
};
export type PipelineOptions = {
  doTrigger(options: { pipelineId }): Promise<void>;
  doSave(pipelineConfig: PipelineDefile): Promise<void>;
  getPipelineDetail(query: { pipelineId }): Promise<PipelineDetail>;
  getHistoryList(query: { pipelineId }): Promise<RunHistory[]>;
  getHistoryLog(query: { historyId }): Promise<RunHistoryLog>;
  getPlugins(): Promise<PluginDefine[]>;
};
