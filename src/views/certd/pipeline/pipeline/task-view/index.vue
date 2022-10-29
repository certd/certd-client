<template>
  <a-modal v-model:visible="taskModal.visible" style="width: 1000px">
    <a-tabs v-model:activeKey="activeKey" tab-position="left" animated>
      <a-tab-pane v-for="item of detail.nodes" :key="item.node.id">
        <template #tab>
          【{{ item.type }}】 {{ item.node.title }}
          <pi-status-show :status="item.node.status.result" type="icon"></pi-status-show>
        </template>
        <div class="pi-task-view-logs">
          <div v-for="(text, index) of item.logs" :key="index">{{ text }}</div>
        </div>
      </a-tab-pane>
    </a-tabs>
  </a-modal>
</template>

<script lang="ts">
import { inject, provide, Ref, ref } from "vue";
import { RunHistory } from "/@/views/certd/pipeline/pipeline/type";
import PiStatusShow from "/@/views/certd/pipeline/pipeline/component/status-show.vue";

export default {
  name: "PiTaskView",
  components: { PiStatusShow },
  props: {},
  setup(props, ctx) {
    const taskModal = ref({
      visible: false
    });

    const detail = ref({ nodes: [] });
    const activeKey = ref();
    const currentHistory: Ref<RunHistory> | undefined = inject("currentHistory");
    const taskViewOpen = (task) => {
      taskModal.value.visible = true;
      activeKey.value = task.id;
      const nodes: any = [];
      nodes.push({
        node: task,
        type: "任务",
        tab: 0,
        logs: [],
        result: {}
      });
      for (let step of task.steps) {
        nodes.push({
          node: step,
          type: "步骤",
          tab: 2,
          logs: []
        });
      }
      for (let node of nodes) {
        if (currentHistory?.value?.logs != null) {
          node.logs = currentHistory.value.logs[node.node.id] || [];
        }
      }
      detail.value = { nodes };
      console.log("nodes", nodes);
    };

    const taskViewClose = () => {
      taskModal.value.visible = false;
    };

    return {
      detail,
      taskModal,
      activeKey,
      taskViewOpen,
      taskViewClose
    };
  }
};
</script>

<style lang="less">
.pi-task-view-logs {
  background-color: #000c17;
  color: #fafafa;
  min-height: 400px;
}
</style>
