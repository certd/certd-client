<template>
  <fs-page class="page-pipeline-edit">
    <template #header>
      <div class="title">
        <fs-editable v-model="pipeline.title" :hover-show="false" :disabled="!editMode"></fs-editable>
      </div>
      <div class="more">
        <template v-if="editMode">
          <a-button type="primary" :loading="saveLoading" @click="save">保存</a-button>
          <a-button class="ml-5" @click="cancel">取消</a-button>
        </template>
        <template v-else>
          <a-button type="primary" @click="edit">编辑</a-button>
        </template>
      </div>
    </template>
    <div class="pipeline-container">
      <div class="pipeline">
        <div class="stages">
          <div class="stage first-stage">
            <div class="title">
              <fs-editable model-value="触发源" :disabled="true" />
            </div>
            <div class="tasks">
              <div class="task-container first-task">
                <div class="line">
                  <div class="flow-line"></div>
                </div>
                <div class="task">
                  <a-button shape="round" type="primary" @click="run">
                    <fs-icon icon="ion:play"></fs-icon>
                    手动触发
                  </a-button>
                </div>
              </div>
              <div v-for="(trigger, index) of pipeline.triggers" :key="trigger.id" class="task-container">
                <div class="line">
                  <div class="flow-line"></div>
                </div>
                <div class="task">
                  <a-button shape="round" @click="triggerEdit(trigger, index)">
                    <fs-icon icon="ion:time"></fs-icon>
                    {{ trigger.title }}
                  </a-button>
                </div>
              </div>

              <div v-if="editMode" class="task-container is-add">
                <div class="line">
                  <div class="flow-line"></div>
                </div>
                <div class="task">
                  <a-button shape="round" type="dashed" @click="triggerAdd">
                    <fs-icon icon="ion:add-circle-outline"></fs-icon>
                    触发源（定时）
                  </a-button>
                </div>
              </div>
            </div>
          </div>

          <div
            v-for="(stage, index) of pipeline.stages"
            :key="stage.id"
            class="stage"
            :class="{ 'last-stage': !editMode && index === pipeline.stages.length - 1 }"
          >
            <div class="title">
              <fs-editable v-model="stage.title" :disabled="!editMode"></fs-editable>
            </div>
            <div class="tasks">
              <div
                v-for="(task, taskIndex) of stage.tasks"
                :key="task.id"
                class="task-container"
                :class="{
                  'first-task': taskIndex === 0
                }"
              >
                <div class="line">
                  <div class="flow-line"></div>
                  <fs-icon
                    v-if="editMode"
                    class="add-stage-btn"
                    title="添加新阶段"
                    icon="ion:add-circle"
                    @click="stageAdd(index)"
                  ></fs-icon>
                </div>
                <div class="task">
                  <a-button shape="round" @click="taskEdit(stage, index, task, taskIndex)">{{ task.title }}</a-button>
                </div>
              </div>
              <div v-if="editMode" class="task-container is-add">
                <div class="line">
                  <div class="flow-line"></div>
                </div>
                <div class="task">
                  <a-button type="dashed" shape="round" @click="taskAdd(stage, index)">
                    <fs-icon class="font-20" icon="ion:add-circle-outline"></fs-icon>
                    并行任务
                  </a-button>
                </div>
              </div>
            </div>
          </div>

          <div v-if="editMode" class="stage last-stage">
            <div class="title">
              <fs-editable model-value="新阶段" :disabled="true" />
            </div>
            <div class="tasks">
              <div class="task-container first-task">
                <div class="line">
                  <div class="flow-line"></div>
                  <fs-icon class="add-stage-btn" title="添加新阶段" icon="ion:add-circle" @click="stageAdd()"></fs-icon>
                </div>
                <div class="task">
                  <a-button shape="round" type="dashed" @click="stageAdd()">
                    <fs-icon icon="ion:add-circle-outline"></fs-icon>
                    新任务
                  </a-button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <task-form ref="taskFormRef" :is-edit="editMode"></task-form>
  </fs-page>
</template>

<script lang="ts">
import { defineComponent, ref, provide, Ref, watch } from "vue";
import TaskForm from "./task-form/index.vue";
import _ from "lodash-es";
import * as api from "../api";
import { notification } from "ant-design-vue";
import { pluginManager } from "/@/views/certd/pipeline/pipeline/plugin";
export type PipelineProps = {
  onSave(pipeline: any): Promise<void>;
  set(pipeline: any);
};
export default defineComponent({
  name: "PipelineEdit",
  components: { TaskForm },
  props: {
    modelValue: {
      type: Object,
      default() {
        return { stages: [] };
      }
    },
    doSave: {
      type: Function,
      default: undefined
    },
    doRun: {
      type: Function,
      default: undefined
    },
    plugins: {
      type: Array,
      default() {
        return [];
      }
    }
  },
  emits: ["update:modelValue"],
  setup(props, ctx) {
    const pipeline: Ref<any> = ref({
      title: "新流水线",
      triggers: [
        {
          id: 0,
          title: "定时每天5点",
          type: "time",
          cron: "* * 5 * *"
        }
      ],
      stages: [
        {
          id: 1,
          title: "证书申请阶段",
          tasks: [{ id: 2, title: "证书申请任务", steps: [{ id: 3, title: "申请证书", type: "CertApply", props: {} }] }]
        },
        {
          id: 2,
          title: "部署证书",
          tasks: [
            {
              id: 2,
              title: "部署到阿里云",
              steps: [{ id: 3, title: "部署到cdn", type: "DeployCertToAliyunCDN", props: {} }]
            },
            {
              id: 2,
              title: "部署到腾讯云",
              steps: [
                { id: 4, title: "上传到腾讯云", type: "UploadCertToTencent", props: {} },
                { id: 5, title: "部署到cdn", type: "DeployCertToTencentCDN", props: {} }
              ]
            }
          ]
        }
      ]
    });

    watch(
      () => {
        return props.modelValue;
      },
      (value) => {
        pipeline.value = value;
      }
    );

    const plugins = ref(props.plugins);
    watch(
      () => {
        return props.plugins;
      },
      (value) => {
        plugins.value = value;
        pluginManager.init(value);
      }
    );

    provide("pipeline", pipeline);
    provide("plugins", plugins);

    const editMode = ref(false);

    function useTask() {
      const taskFormRef: Ref<any> = ref(null);
      const currentStageIndex = ref(0);
      const currentTaskIndex = ref(0);
      provide("currentStageIndex", currentStageIndex);
      provide("currentTaskIndex", currentTaskIndex);
      const taskAdd = (stage: any, stageIndex: number, onSuccess) => {
        currentStageIndex.value = stageIndex;
        currentTaskIndex.value = stage.tasks.length;
        taskFormRef.value.taskAdd((type, value) => {
          if (type === "save") {
            stage.tasks.push(value);
            if (onSuccess) {
              onSuccess();
            }
          }
        });
      };
      const taskEdit = (stage, stageIndex, task, taskIndex, onSuccess) => {
        currentStageIndex.value = stageIndex;
        currentTaskIndex.value = taskIndex;
        if (taskFormRef.value == null) {
          return;
        }
        if (editMode.value) {
          taskFormRef.value.taskEdit(task, (type, value) => {
            if (type === "delete") {
              stage.tasks.splice(taskIndex, 1);
              if (stage.tasks.length === 0) {
                _.remove(pipeline.value.stages, (item) => {
                  return item.id === stage.id;
                });
              }
            } else if (type === "save") {
              stage.tasks[taskIndex] = value;
            }
            if (onSuccess) {
              onSuccess(type);
            }
          });
        } else {
          taskFormRef.value.taskView(task, (type, value) => {});
        }
      };

      return { taskAdd, taskEdit, taskFormRef };
    }

    function useStage(useTaskRet) {
      const stageAdd = (stageIndex = pipeline.value.stages.length) => {
        const stage = {
          title: "新阶段",
          tasks: []
        };
        //stage: any, stageIndex: number, onSuccess
        useTaskRet.taskAdd(stage, stageIndex, () => {
          let task = stage.tasks[0] as any;
          stage.title = task.title + "阶段";
          //插入阶段
          pipeline.value.stages.splice(stageIndex, 0, stage);
        });
      };
      return {
        stageAdd
      };
    }

    function useTrigger() {
      const triggerAdd = () => {};
      const triggerEdit = (trigger, index) => {};
      return {
        triggerAdd,
        triggerEdit
      };
    }

    function useActions() {
      const backup = ref();
      const saveLoading = ref();
      const run = async () => {
        if (editMode.value) {
          notification.warn({ message: "请先保存，再运行管道" });
          return;
        }
        if (props.doRun) {
          await props.doRun(pipeline.value);
          notification.success({ message: "管道已经开始运行" });
        }
      };
      const save = async () => {
        saveLoading.value = true;
        try {
          if (props.doSave) {
            await props.doSave(pipeline.value);
          }
          ctx.emit("update:modelValue", pipeline.value);
          editMode.value = false;
        } finally {
          saveLoading.value = false;
        }
      };
      const edit = () => {
        backup.value = _.cloneDeep(pipeline.value);
        editMode.value = true;
      };
      const cancel = () => {
        pipeline.value = backup.value;
        editMode.value = false;
      };
      return {
        run,
        save,
        edit,
        cancel,
        saveLoading
      };
    }

    const useTaskRet = useTask();
    const useStageRet = useStage(useTaskRet);

    return {
      pipeline,
      editMode,
      ...useTaskRet,
      ...useStageRet,
      ...useTrigger(),
      ...useActions()
    };
  }
});
</script>
<style lang="less">
.page-pipeline-edit {
  .fs-page-header {
    .title {
      .fs-editable {
        width: 300px;
      }
    }
  }
  .pipeline-container {
    width: 100%;
    height: 100%;
    position: relative;
    background-color: #f0f0f0;
  }
  .pipeline {
    position: absolute;
    left: 0;
    top: 0;
    height: 100%;
    background-color: #f0f0f0;
    .stages {
      padding-left: 20px;
      display: flex;
      overflow: auto;
      min-width: 100%;
      height: 100%;
      .stage {
        width: 300px;
        border-right: 1px solid #c7c7c7;
        .is-add {
          visibility: hidden;
          color: gray;
        }
        &:hover .is-add {
          visibility: visible;
        }

        .title {
          padding: 20px;
          color: gray;
        }
        &.first-stage {
          .line {
            width: 50% !important;
            .flow-line {
              border-left: 0;
            }
          }
        }
        &.last-stage {
          .line {
            width: 50% !important;
            left: 0;
            right: auto;
            .flow-line {
              border-right: 0;
            }
            .add-stage-btn {
              visibility: hidden;
            }
          }
        }

        .line {
          height: 50px;
          position: absolute;
          top: -25px;
          right: 0;
          width: 100%;
          .flow-line {
            height: 100%;
            margin-left: 28px;
            margin-right: 28px;
            border: 1px solid #c7c7c7;
            border-top: 0;
          }
          .add-stage-btn {
            display: inline-flex;
            visibility: hidden;
            font-size: 24px;
            cursor: pointer;
            position: absolute;
            bottom: -12px;
            left: -12px;
            z-index: 100;
            &:hover {
              color: #1890ff;
            }
          }
        }

        .tasks {
          .task-container {
            width: 100%;
            height: 50px;
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            position: relative;
            &.first-task {
              .line {
                .flow-line {
                  margin: 0;
                  border-left: 0;
                  border-right: 0;
                }
                .add-stage-btn {
                  visibility: visible;
                }
              }
            }
            .task {
              display: flex;
              flex-direction: column;
              justify-content: center;
              align-items: center;
              height: 100%;
              z-index: 2;

              .ant-btn {
                width: 200px;
              }
            }
          }
        }
      }
    }
  }
}
</style>
