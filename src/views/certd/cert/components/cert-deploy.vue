<template>
  <a-page-header class="cert-deploy" title="自动部署" sub-title="证书申请成功后，自动运行以下部署任务">
    <template #extra>
      <template v-if="isEdit">
        <a-button key="add" type="primary" @click="deployAdd">添加流程</a-button>
        <a-button key="save" type="success" @click="save">保存</a-button>
        <a-button key="cancel" type="default" @click="cancel">取消</a-button>
      </template>
      <template v-else>
        <a-button key="play" type="primary" @click="play">重新部署</a-button>
        <a-button key="edit" type="primary" @click="edit">编辑</a-button>
      </template>
    </template>
    <div class="flow-deploy">
      <a-card v-for="(deploy, index) of deployRef" :key="index" class="deploy-item" size="small">
        <template #title>
          <div class="deploy-name">
            <template v-if="deploy._isEdit">
              <a-input
                v-model:value="deploy.title"
                :validate-status="deploy.title ? '' : 'error'"
                placeholder="请输入流程名称"
                @keyup.enter="deployCloseEditMode(deploy)"
                @blur="deployCloseEditMode(deploy)"
              >
                <template #suffix>
                  <CheckOutlined style="color: rgba(0, 0, 0, 0.45)" @click="deployCloseEditMode(deploy)" />
                </template>
              </a-input>
            </template>
            <template v-else>
              <span> <NodeIndexOutlined /> {{ deploy.title }}</span>
              <EditOutlined v-if="isEdit" class="ml-10 edit-icon" @click="deployOpenEditMode(deploy)" />
            </template>
          </div>
        </template>
        <template #extra>
          <a-button v-if="isEdit" type="danger" @click="deployDelete(deploy, index)">
            <template #icon><DeleteOutlined /></template>
          </a-button>
        </template>
        <div class="task-list">
          <div class="task-item-wrapper">
            <a-button class="task-item" shape="round"> 开始 </a-button>
          </div>
          <div v-for="(task, iindex) of deploy.tasks" :key="iindex" class="task-item-wrapper">
            <ArrowRightOutlined class="task-next-icon" />
            <a-button class="task-item" shape="round" @click="taskEdit(deploy, task, index)">
              <ThunderboltOutlined />
              {{ task.title }}
            </a-button>
          </div>
          <div v-if="isEdit" class="task-item-wrapper">
            <ArrowRightOutlined class="task-next-icon" />
            <a-button type="primary" class="task-item" shape="round" @click="taskAdd(deploy)">
              <PlusOutlined />
              添加新任务
            </a-button>
          </div>
        </div>
      </a-card>
    </div>
    <task-form ref="taskFormRef"></task-form>
  </a-page-header>
</template>

<script>
import { computed, defineComponent, reactive, ref, watch } from "vue";
import TaskForm from "./task-form/index.vue";
function useDeploy(deployRef) {
  const deployAdd = () => {
    deployRef.value.push({
      title: `D${deployRef.value.length + 1}-新部署流程`,
      _isEdit: false,
      tasks: []
    });
  };

  const deployCloseEditMode = (deploy) => {
    if (!deploy.title) {
      message.error("请输入流程名称");
      return;
    }
    deploy._isEdit = false;
  };

  const deployOpenEditMode = (deploy) => {
    deploy._isEdit = true;
  };

  const deployDelete = (deploy, index) => {
    deployRef.value.splice(index, 1);
  };

  return {
    deployAdd,
    deployCloseEditMode,
    deployOpenEditMode,
    deployDelete
  };
}

function useTask(isEdit) {
  const taskFormRef = ref(null);
  const taskAdd = (deploy) => {
    taskFormRef.value.taskAdd(deploy);
  };
  const taskEdit = (deploy, task, index) => {
    if (isEdit) {
      taskFormRef.value.taskEdit(deploy, task, index);
    }
  };
  return { taskAdd, taskEdit, taskFormRef };
}
export default defineComponent({
  name: "CertDeploy",
  components: { TaskForm },
  props: {
    modelValue: {
      type: Object,
      default: null
    }
  },
  emits: ["update:modelValue"],
  setup(props, ctx) {
    const deployRef = ref([]);
    const mode = ref("view");
    watch(
      () => {
        return props.modelValue;
      },
      (value) => {
        deployRef.value = value || [];
      }
    );

    const isEdit = computed(() => {
      return mode.value === "edit";
    });

    function play() {}
    function edit() {
      mode.value = "edit";
    }
    function save() {
      mode.value = "view";
      ctx.emit("update:modelValue", deployRef.value);
    }
    function cancel() {
      mode.value = "view";
      deployRef.value = props.modelValue || [];
    }

    return {
      deployRef,
      mode,
      play,
      edit,
      save,
      cancel,
      isEdit,
      ...useDeploy(deployRef),
      ...useTask()
    };
  }
});
</script>
<style lang="less">
.cert-deploy {
  .task-item {
    .ant-card-bordered {
      border-color: #00aaaa;
    }
  }

  .flow-deploy {
    flex-shrink: 0;
    flex: 1;
    display: flex;
    flex-direction: column;

    .deploy-item {
      margin-bottom: 10px;
    }
    // min-width:70%;
    .deploy-name {
      max-width: 300px;

      .edit-icon {
        color: #737070;
      }
    }
  }

  .task-list {
    display: flex;
    flex-direction: row;
    flex-wrap: wrap;
    justify-content: flex-start;
    align-items: center;

    > * {
      margin-bottom: 10px;
    }

    .task-item-wrapper {
      display: flex;
      flex-direction: row;
      flex-wrap: nowrap;
      justify-content: flex-start;
      align-items: center;
    }

    .task-item {
      //border: 1px solid #eee;
      //padding: 10px 20px;
      //border-radius: 20px;
    }

    .task-add-icon {
      font-size: 24px;
      margin-right: 10px;
    }

    .task-next-icon {
      margin-left: 10px;
      margin-right: 10px;
    }
  }
}
</style>
