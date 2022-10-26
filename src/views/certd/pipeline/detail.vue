<template>
  <fs-page class="fs-pipeline-detail">
    <pipeline :model-value="pipeline" :do-save="doSave" :do-run="doRun" :plugins="plugins"></pipeline>
  </fs-page>
</template>

<script lang="ts">
import { defineComponent, Ref, ref } from "vue";
import Pipeline from "./pipeline/index.vue";
import * as pluginApi from "./api.plugin";
import * as api from "./api";
import { useRoute } from "vue-router";

export default defineComponent({
  name: "PipelineDetail",
  components: { Pipeline },
  setup() {
    const route = useRoute();
    const pipeline: Ref<any> = ref({});
    const pipelineId = route.query.id;
    const loadDetail = async () => {
      pipeline.value = await api.GetDetail(pipelineId);
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
    return {
      plugins,
      pipeline,
      doSave,
      doRun
    };
  }
});
</script>
<style lang="less">
.page-pipeline-detail {
}
</style>
