<template>
  <fs-page class="fs-pipeline-detail">
    <pipeline-edit v-model:edit-mode="editMode" :pipeline-id="pipelineId" :options="pipelineOptions"></pipeline-edit>
  </fs-page>
</template>

<script lang="ts">
import { defineComponent, Ref, ref } from "vue";
import PipelineEdit from "./pipeline/index.vue";
import * as pluginApi from "./api.plugin";
import * as historyApi from "./api.history";
import * as api from "./api";
import { useRoute } from "vue-router";
import {
  PipelineDefine,
  PipelineDetail,
  PipelineOptions,
  RunHistory,
  RunHistoryLog
} from "/@/views/certd/pipeline/pipeline/type";
import { PluginDefine } from "@certd/pipeline/src";

export default defineComponent({
  name: "PipelineDetail",
  components: { PipelineEdit },
  setup() {
    const route = useRoute();
    const pipelineId = ref(route.query.id);

    const getPipelineDetail = async ({ pipelineId }) => {
      const detail = await api.GetDetail(pipelineId);
      return {
        pipeline: {
          id: detail.pipeline.id,
          stages: [],
          triggers: [],
          ...JSON.parse(detail.pipeline.content || "{}")
        }
      } as PipelineDetail;
    };

    const getHistoryList = async ({ pipelineId }) => {
      const list: RunHistory[] = await historyApi.GetList({ pipelineId });
      return list;
    };

    const getHistoryLog = async ({ historyId }) => {
      return await historyApi.GetLogs(historyId);
    };

    const getPlugins = async () => {
      const plugins = await pluginApi.GetList({});
      return plugins as PluginDefine[];
    };

    async function doSave(pipelineConfig: PipelineDefine) {
      await api.Save({
        id: pipelineConfig.id,
        content: JSON.stringify(pipelineConfig)
      });
    }
    async function doTrigger({ pipelineId }) {
      await api.Trigger(pipelineId);
    }

    const pipelineOptions: Ref<PipelineOptions> = ref({
      doTrigger,
      doSave,
      getPlugins,
      getHistoryList,
      getHistoryLog,
      getPipelineDetail
    });

    const editMode = ref(false);
    if (route.query.editMode !== "false") {
      editMode.value = true;
    }

    return {
      pipelineOptions,
      pipelineId,
      editMode
    };
  }
});
</script>
<style lang="less">
.page-pipeline-detail {
}
</style>
