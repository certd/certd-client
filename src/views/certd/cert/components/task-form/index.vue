<template>
  <a-drawer
    v-model:visible="taskDrawerVisible"
    placement="right"
    :closable="true"
    width="600px"
    :after-visible-change="taskDrawerOnAfterVisibleChange"
  >
    <template #title>
      编辑任务
      <a-button v-if="mode === 'edit'" @click="taskDelete()">
        <template #icon><DeleteOutlined /></template>
      </a-button>
    </template>
    <template v-if="currentTask">
      <d-container v-if="currentTask._isAdd" class="task-edit-form">
        <a-row :gutter="10">
          <a-col v-for="(item, index) of taskPluginDefineList" :key="index" class="task-plugin" :span="12">
            <a-card
              hoverable
              :class="{ current: item.name === currentTask.type }"
              @click="taskTypeSelected(item)"
              @dblclick="
                taskTypeSelected(item);
                taskTypeSave();
              "
            >
              <a-card-meta>
                <template #title>
                  <a-avatar :src="item.icon || '/images/plugin.png'" />
                  <span class="title">{{ item.title }}</span>
                </template>
                <template #description>
                  <span :title="item.desc">{{ item.desc }}</span>
                </template>
              </a-card-meta>
            </a-card>
          </a-col>
        </a-row>
        <a-button v-if="isEdit" type="primary" @click="taskTypeSave"> 确定 </a-button>
      </d-container>
      <d-container v-else class="d-container">
        <a-form
          ref="taskFormRef"
          class="task-form"
          :model="currentTask"
          :label-col="labelCol"
          :wrapper-col="wrapperCol"
        >
          <fs-form-item
            v-model="currentTask.title"
            :item="{
              title: '任务名称',
              key: 'title',
              component: {
                name: 'a-input',
                vModel: 'value'
              },
              rules: [{ required: true, message: '此项必填' }]
            }"
            :get-context-fn="blankFn"
          />
          <template v-for="(item, key) in currentPlugin.input" :key="key">
            <fs-form-item v-model="currentTask.props[key]" :item="item" :get-context-fn="blankFn" />
          </template>
        </a-form>

        <template #footer>
          <a-form-item v-if="isEdit" :wrapper-col="{ span: 14, offset: 4 }">
            <a-button type="primary" @click="taskSave"> 确定 </a-button>
          </a-form-item>
        </template>
      </d-container>
    </template>
  </a-drawer>
</template>

<script>
import { message } from "ant-design-vue";
import * as pluginsApi from "./api";
import { ref } from "vue";
import _ from "lodash-es";
import { nanoid } from "nanoid";

/**
 *  task drawer
 * @returns
 */
function useTaskForm(context) {
  const taskPluginDefineList = ref([]);
  const onCreated = async () => {
    const plugins = await pluginsApi.GetList();
    taskPluginDefineList.value = plugins;
  };

  onCreated();

  const mode = ref("add");
  const callback = ref();
  const currentTask = ref({ title: undefined, props: {} });
  const currentPlugin = ref(null);
  const taskFormRef = ref(null);
  const taskDrawerVisible = ref(false);
  const rules = ref({
    name: [
      {
        type: "string",
        required: true,
        message: "请输入名称"
      }
    ]
  });

  const taskTypeSelected = (item) => {
    currentTask.value.type = item.name;
    currentTask.value.title = item.title;
    console.log("currentTaskTypeChanged:", currentTask.value);
  };

  const taskTypeSave = () => {
    currentTask.value._isAdd = false;
    if (currentTask.value.type == null) {
      message.warn("请先选择类型");
      return;
    }
    // 给task的input设置默认值
    changeCurrentPlugin(currentTask.value);

    if (currentTask.value.props) {
      currentTask.value.props = {};
    }
    for (const key in currentPlugin.value.input) {
      const input = currentPlugin.value.input[key];
      if (input.default != null) {
        currentTask.value.props[key] = input.default;
      }
    }
  };

  const taskDrawerShow = () => {
    taskDrawerVisible.value = true;
  };
  const taskDrawerClose = () => {
    taskDrawerVisible.value = false;
  };

  const taskDrawerOnAfterVisibleChange = (val) => {
    console.log("taskDrawerOnAfterVisibleChange", val);
  };

  const taskOpen = (task, emit) => {
    callback.value = emit;
    currentTask.value = _.cloneDeep(task);
    console.log("currentTaskOpen", currentTask.value);
    if (task.type) {
      changeCurrentPlugin(currentTask.value);
    }
    taskDrawerShow();
  };

  const taskAdd = (emit) => {
    mode.value = "add";
    const task = { id: nanoid(), title: "新任务", type: undefined, _isAdd: true, props: {} };
    taskOpen(task, emit);
  };

  const taskEdit = (task, emit) => {
    mode.value = "edit";
    taskOpen(task, emit);
  };

  const taskView = (task, emit) => {
    mode.value = "view";
    taskOpen(task, emit);
  };

  const changeCurrentPlugin = (task) => {
    const taskType = task.type;
    const curPlug = taskPluginDefineList.value.find((p) => {
      return p.name === taskType;
    });
    if (curPlug) {
      task.type = taskType;
      task._isAdd = false;
      currentPlugin.value = curPlug;
    }

    console.log("currentTaskTypeChanged:", currentTask.value, currentPlugin.value);
  };

  const taskSave = async (e) => {
    console.log("currentTaskSave", currentTask.value);
    try {
      await taskFormRef.value.validate();
    } catch (e) {
      console.error("表单验证失败:", e);
      return;
    }

    callback.value("save", currentTask.value);
    taskDrawerClose();
  };

  const taskDelete = () => {
    callback.value("delete");
    taskDrawerClose();
  };

  const blankFn = () => {
    return {};
  };
  return {
    taskTypeSelected,
    taskTypeSave,
    taskPluginDefineList,
    taskFormRef,
    mode,
    taskAdd,
    taskEdit,
    taskView,
    taskDrawerShow,
    taskDrawerVisible,
    taskDrawerOnAfterVisibleChange,
    currentTask,
    currentPlugin,
    taskSave,
    taskDelete,
    rules,
    blankFn
  };
}

function useProviderManager() {
  const providerManager = ref(null);
  const providerManagerOpen = () => {
    providerManager.value.open();
  };
  return { providerManager, providerManagerOpen };
}
export default {
  name: "TaskForm",
  props: {
    isEdit: {
      type: Boolean,
      default: true
    }
  },
  emits: ["update"],
  setup(props, context) {
    return {
      ...useTaskForm(context),
      ...useProviderManager(),
      labelCol: { span: 6 },
      wrapperCol: { span: 16 }
    };
  }
};
</script>

<style lang="less">
.task-edit-form {
  .body {
    padding: 10px;
    .ant-card {
      margin-bottom: 10px;

      &.current {
        border-color: #00b7ff;
      }

      .ant-card-meta-title {
        display: flex;
        flex-direction: row;
        justify-content: flex-start;
      }

      .ant-avatar {
        width: 24px;
        height: 24px;
        flex-shrink: 0;
      }

      .title {
        margin-left: 5px;
        white-space: nowrap;
        flex: 1;
        display: block;
        overflow: hidden;
        text-overflow: ellipsis;
      }
    }

    .ant-card-body {
      padding: 14px;
      height: 100px;

      overflow-y: hidden;

      .ant-card-meta-description {
        font-size: 10px;
        line-height: 20px;
        height: 40px;
        color: #7f7f7f;
      }
    }
  }
}
</style>
