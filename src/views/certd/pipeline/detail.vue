<template>
  <fs-page class="fs-pipeline-detail">
    <pipeline-edit
      v-model:edit-mode="editMode"
      v-model:model-value="pipeline"
      :do-save="doSave"
      :do-run="doRun"
      :plugins="plugins"
    ></pipeline-edit>
  </fs-page>
</template>

<script lang="ts">
import { defineComponent, Ref, ref } from "vue";
import PipelineEdit from "./pipeline/index.vue";
import * as pluginApi from "./api.plugin";
import * as api from "./api";
import { useRoute } from "vue-router";

export default defineComponent({
  name: "PipelineDetail",
  components: { PipelineEdit },
  setup() {
    const route = useRoute();
    const pipeline: Ref<any> = ref({});
    const pipelineId = route.query.id;
    const loadDetail = async () => {
      const detail = await api.GetDetail(pipelineId);
      pipeline.value = {
        id: detail.pipeline.id,
        stages: [],
        triggers: [],
        ...JSON.parse(detail.pipeline.content || "{}")
      };
    };
    loadDetail();

    const plugins: Ref<any[]> = ref([]);
    const loadPlugin = async () => {
      plugins.value = await pluginApi.GetList({});
    };
    loadPlugin();

    async function doSave(pipelineConfig) {
      pipeline.value = pipelineConfig;
      await api.Save({
        id: pipeline.value.id,
        content: JSON.stringify(pipelineConfig)
      });
    }
    async function doRun() {
      await api.Run(pipeline.value.id);
    }

    const editMode = ref(false);
    if (route.query.editMode !== "false") {
      editMode.value = true;
    }

    return {
      plugins,
      pipeline,
      doSave,
      doRun,
      editMode
    };
  }
});
</script>
<style lang="less">
.page-pipeline-detail {
}
</style>
