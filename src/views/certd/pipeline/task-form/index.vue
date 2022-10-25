<template>
  <a-drawer
    v-model:visible="taskDrawerVisible"
    placement="right"
    :closable="true"
    width="600px"
    class="task-edit-form"
    :after-visible-change="taskDrawerOnAfterVisibleChange"
  >
    <template #title>
      编辑任务
      <a-button v-if="mode === 'edit'" @click="taskDelete()">
        <template #icon><DeleteOutlined /></template>
      </a-button>
    </template>
    <template v-if="currentTask">
      <d-container class="d-container">
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

          <div class="steps">
            <a-descriptions title="任务步骤" size="small">
              <template #extra>
                <a-button type="primary" @click="stepAdd(currentTask)">添加步骤</a-button>
              </template>
            </a-descriptions>
            <a-list class="step-list" item-layout="horizontal" :data-source="currentTask.steps">
              <template #renderItem="{ item, index }">
                <a-list-item>
                  <template #actions>
                    <a key="edit" @click="stepEdit(currentTask, item, index)">编辑</a>
                  </template>
                  <a-list-item-meta>
                    <template #title>
                      {{ item.title }}
                    </template>
                    <template #avatar>
                      <fs-icon icon="ion:flash"></fs-icon>
                    </template>
                  </a-list-item-meta>
                </a-list-item>
              </template>
            </a-list>
          </div>
        </a-form>

        <step-form ref="stepFormRef" :is-edit="true"></step-form>

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
import { ref } from "vue";
import _ from "lodash-es";
import { nanoid } from "nanoid";
import StepForm from "../step-form/index.vue";

function useStep(isEdit) {
  const stepFormRef = ref(null);
  const stepAdd = (task) => {
    stepFormRef.value.stepAdd((type, value) => {
      if (type === "save") {
        task.steps.push(value);
      }
    });
  };
  const stepEdit = (task, step, index) => {
    console.log("step.edit start", task, step, isEdit);
    if (isEdit) {
      console.log("step.edit", task, step);
      stepFormRef.value.stepEdit(step, (type, value) => {
        console.log("step.save", step, type, value);
        if (type === "delete") {
          task.steps.splice(index, 1);
        } else if (type === "save") {
          task.steps[index] = { ...value };
        }
        console.log("task.steps", task.steps);
      });
    } else {
      stepFormRef.value.stepView(step, (type, value) => {});
    }
  };

  return { stepAdd, stepEdit, stepFormRef };
}

/**
 *  task drawer
 * @returns
 */
function useTaskForm(context) {
  const mode = ref("add");
  const callback = ref();
  const currentTask = ref({ title: undefined, steps: [] });
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
    taskDrawerShow();
  };

  const taskAdd = (emit) => {
    mode.value = "add";
    const task = { id: nanoid(), title: "新任务", steps: [] };
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
    taskFormRef,
    mode,
    taskAdd,
    taskEdit,
    taskView,
    taskDrawerShow,
    taskDrawerVisible,
    taskDrawerOnAfterVisibleChange,
    currentTask,
    taskSave,
    taskDelete,
    rules,
    blankFn,
    ...useStep(true)
  };
}

export default {
  name: "TaskForm",
  components: { StepForm },
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
      labelCol: { span: 6 },
      wrapperCol: { span: 16 },
      ...useStep(props.isEdit)
    };
  }
};
</script>

<style lang="less">
.task-edit-form {
  .steps {
    margin: 0 50px 0 50px;
  }
}
</style>
