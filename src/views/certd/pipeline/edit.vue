<template>
  <fs-page class="page-pipeline-edit">
    <template #header>
      <div class="title">
        <hover-edit v-model="pipeline.title"></hover-edit>
      </div>
      <div class="more">
        <a-button>保存</a-button>
        <a-button class="ml-5">保存并运行</a-button>
      </div>
    </template>
    <div class="pipeline">
      <div class="stages">
        <div
          v-for="(stage, index) of pipeline.stages"
          :key="stage.id"
          class="stage"
          :class="{ 'first-stage': index === 0 }"
        >
          <div class="title">
            <hover-edit v-model="stage.title"></hover-edit>
          </div>
          <div class="tasks">
            <div
              v-for="(task, taskIndex) of stage.tasks"
              :key="task.id"
              class="task-container"
              :class="{ 'first-task': taskIndex === 0 }"
            >
              <div class="line">
                <div class="flow-line"></div>
                <fs-icon class="add-stage-btn" title="添加新阶段" icon="ion:add-circle"></fs-icon>
              </div>
              <div class="task">
                <a-button shape="round" @click="taskEdit(stage, task, taskIndex)">{{ task.title }}</a-button>
              </div>
            </div>
            <div class="task-container is-add">
              <div class="line">
                <div class="flow-line"></div>
              </div>
              <div class="task">
                <a-button type="dashed" shape="round" @click="taskAdd(stage)">
                  <fs-icon class="font-20" icon="ion:add-circle-outline"></fs-icon>
                  并行任务
                </a-button>
              </div>
            </div>
          </div>
        </div>

        <div class="stage last-stage">
          <div class="title">
            <hover-edit model-value="新阶段" :disabled="true" />
          </div>
          <div class="tasks">
            <div class="task-container first-task">
              <div class="line">
                <div class="flow-line"></div>
              </div>
              <div class="task">
                <a-button shape="round" type="dashed">
                  <fs-icon style="font-size: 20px" icon="ion:add-circle-outline"></fs-icon>
                  新任务
                </a-button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <task-form ref="taskFormRef" :is-edit="true"></task-form>
  </fs-page>
</template>

<script>
import { defineComponent, ref, onMounted } from "vue";
import TaskForm from "./task-form/index.vue";
function useTask(isEdit = true) {
  const taskFormRef = ref(null);
  const taskAdd = (stage) => {
    taskFormRef.value.taskAdd((type, value) => {
      if (type === "save") {
        stage.tasks.push(value);
      }
    });
  };
  const taskEdit = (stage, task, index) => {
    if (isEdit) {
      taskFormRef.value.taskEdit(task, (type, value) => {
        if (type === "delete") {
          stage.tasks.splice(index, 1);
        } else if (type === "save") {
          stage.tasks[index] = value;
        }
      });
    } else {
      taskFormRef.value.taskView(task, (type, value) => {});
    }
  };

  return { taskAdd, taskEdit, taskFormRef };
}

export default defineComponent({
  name: "PipelineEdit",
  components: { TaskForm },
  setup() {
    const pipeline = ref({
      title: "新流水线",
      stages: [
        {
          id: 1,
          title: "证书申请阶段",
          tasks: [{ id: 2, title: "证书申请任务", steps: [{ id: 3, title: "申请证书", type: "certApply", props: {} }] }]
        },
        {
          id: 2,
          title: "部署证书",
          tasks: [
            {
              id: 2,
              title: "部署到阿里云",
              steps: [{ id: 3, title: "部署到cdn", type: "deployCertToAliyunCDN", props: {} }]
            },
            {
              id: 2,
              title: "部署到腾讯云",
              steps: [
                { id: 4, title: "上传到腾讯云", type: "uploadCertToTencent", props: {} },
                { id: 5, title: "部署到cdn", type: "deployCertToTencentCDN", props: {} }
              ]
            }
          ]
        }
      ]
    });
    return {
      pipeline,
      ...useTask()
    };
  }
});
</script>
<style lang="less">
.page-pipeline-edit {
  .fs-page-header {
    .title {
      .hover-edit {
        width: 300px;
      }
    }
  }
  .pipeline {
    width: 100%;
    height: 100%;
    position: relative;
    .stages {
      position: absolute;
      left: 0;
      top: 0;
      height: 100%;
      background-color: #f0f0f0;
      display: flex;
      overflow-x: auto;
      min-width: 100%;
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
            .add-stage-btn {
              display: none;
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
            display: none;
            font-size: 24px;
            cursor: pointer;
            position: absolute;
            bottom: -12.5px;
            right: -12.5px;
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
                  display: block;
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
