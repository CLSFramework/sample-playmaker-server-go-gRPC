package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
    "sync/atomic"
	pb "playmaker-server-go-gRPC/service_pb"
	"google.golang.org/grpc"
	"math"
	"net"
)

// GameHandler implements the GameServiceServer interface.
type GameHandler struct {
	pb.UnimplementedGameServer
	sharedLock                  *sync.Mutex
	sharedNumberOfConnections    *int32
	agents                      map[int32]*GrpcAgent
	serverParams                *pb.ServerParam
	playerParams                *pb.PlayerParam
	playerTypes                 map[int32]*pb.PlayerType
}

type GrpcAgent struct {
	agentType  pb.AgentType
	uniformNumber int32
	logger     *log.Logger
    debugMode                 bool
}
func (g *GameHandler) GetCoachActions(context context.Context, state *pb.State) (*pb.CoachActions, error) {
    actions := []*pb.CoachAction{}
    actions = append(actions, &pb.CoachAction{
        Action: &pb.CoachAction_DoHeliosSubstitute{DoHeliosSubstitute: &pb.DoHeliosSubstitute{}},
    })
    return &pb.CoachActions{Actions: actions}, nil
}
func (g *GameHandler) GetTrainerActions(context context.Context, state *pb.State) (*pb.TrainerActions, error) {
    actions := []*pb.TrainerAction{}
    actions = append(actions, &pb.TrainerAction{
        Action: &pb.TrainerAction_DoMoveBall{DoMoveBall: &pb.DoMoveBall{Position: &pb.RpcVector2D{X: 0, Y: 0} , Velocity: &pb.RpcVector2D{X: 0, Y: 0}}},
    })
    return &pb.TrainerActions{Actions: actions}, nil
}
func (g *GameHandler) GetPlayerActions(context context.Context, state *pb.State) (*pb.PlayerActions, error) {
	g.debugLog(fmt.Sprintf("================================= cycle=%d.%d =================================", state.WorldModel.Cycle, state.WorldModel.StopedCycle),state.RegisterResponse.UniformNumber)
	fmt.Printf("register response %v\n", state.RegisterResponse.UniformNumber)
	actions := []*pb.PlayerAction{}
	if state.WorldModel.GameModeType == pb.GameModeType_PlayOn {
		if state.WorldModel.Self.IsGoalie {
			actions = append(actions, &pb.PlayerAction{
				Action: &pb.PlayerAction_HeliosGoalie{HeliosGoalie: &pb.HeliosGoalie{}},
			})
		} else if state.WorldModel.Self.IsKickable {
			actions = append(actions, &pb.PlayerAction{
				Action: &pb.PlayerAction_HeliosOffensivePlanner{HeliosOffensivePlanner: &pb.HeliosOffensivePlanner{
					LeadPass:       true,
					DirectPass:     true,
					ThroughPass:    true,
					SimplePass:     true,
					ShortDribble:   true,
					LongDribble:    true,
					SimpleShoot:    true,
					SimpleDribble:  true,
					Cross:          true,
					ServerSideDecision: true,
				}},
			})
			actions = append(actions, &pb.PlayerAction{
				Action: &pb.PlayerAction_HeliosShoot{HeliosShoot: &pb.HeliosShoot{}},
			})
		} else {
			actions = append(actions, &pb.PlayerAction{
				Action: &pb.PlayerAction_HeliosBasicMove{HeliosBasicMove: &pb.HeliosBasicMove{}},
			})
		}
	} else {
		actions = append(actions, &pb.PlayerAction{
			Action: &pb.PlayerAction_HeliosSetPlay{HeliosSetPlay: &pb.HeliosSetPlay{}},
		})
	}
	g.debugLog(fmt.Sprintf("Actions: %v", actions),state.RegisterResponse.UniformNumber)
	return &pb.PlayerActions{Actions: actions}, nil
}

func (g *GameHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	g.sharedLock.Lock()
	defer g.sharedLock.Unlock()

	clientId := atomic.AddInt32(g.sharedNumberOfConnections, 1)
    fmt.Printf("Registering agent %d \n", req.UniformNumber)

	logFileName := fmt.Sprintf("logs/%d.log", req.UniformNumber)
	logFile, err := os.Create(logFileName)
	if err != nil {
		return nil, fmt.Errorf("could not create log file: %v", err)
	}
    g.debugLog(fmt.Sprintf("Registering agent %d", clientId),clientId)
	logger := log.New(logFile, "INFO: ", log.LstdFlags)

	agent := &GrpcAgent{
		agentType:    req.AgentType,
		uniformNumber: req.UniformNumber,  
		logger:       logger,
	}

	g.agents[clientId] = agent

	g.debugLog(fmt.Sprintf("Agent %d registered with uniform number: %d", clientId, req.UniformNumber),req.UniformNumber)

	return &pb.RegisterResponse{
		ClientId:   clientId,      
		TeamName:   req.TeamName,
		UniformNumber: req.UniformNumber,  
		AgentType:  req.AgentType,
	}, nil
}


func (g *GameHandler) SendServerParams(ctx context.Context, params *pb.ServerParam) (*pb.Empty, error) {
	g.sharedLock.Lock()
	defer g.sharedLock.Unlock()
	g.serverParams = params
	return &pb.Empty{}, nil
}

func (g *GameHandler) SendPlayerParams(ctx context.Context, params *pb.PlayerParam) (*pb.Empty, error) {
	g.sharedLock.Lock()
	defer g.sharedLock.Unlock()
	g.playerParams = params
	return &pb.Empty{}, nil
}

func (g *GameHandler) SendPlayerType(ctx context.Context, playerType *pb.PlayerType) (*pb.Empty, error) {
    g.sharedLock.Lock() 
    defer g.sharedLock.Unlock() 

    g.playerTypes[playerType.Id] = playerType


    return &pb.Empty{}, nil
}

func (g *GameHandler) SendInitMessage(ctx context.Context, initMessage *pb.InitMessage) (*pb.Empty, error) {
	g.sharedLock.Lock()
	defer g.sharedLock.Unlock()
	g.agents[initMessage.RegisterResponse.UniformNumber].debugMode = initMessage.DebugMode
	return &pb.Empty{}, nil
}

func (g *GameHandler) SendByeCommand(ctx context.Context, resp *pb.RegisterResponse) (*pb.Empty, error) {
	g.sharedLock.Lock()
	defer g.sharedLock.Unlock()
	delete(g.agents, resp.ClientId)
	return &pb.Empty{}, nil
}

func (g *GameHandler) debugLog(message string,uniformNumber int32) {
	for _, agent := range g.agents {
		if agent.logger != nil && uniformNumber == agent.uniformNumber {
			agent.logger.Println(message)
		}
	}
}

func (g *GameHandler) GetBestPlannerAction(ctx context.Context, request *pb.BestPlannerActionRequest) (*pb.BestPlannerActionResponse, error) {
	g.sharedLock.Lock()
	defer g.sharedLock.Unlock()
    g.debugLog(fmt.Sprintf("GetBestPlannerAction cycle:%d pairs:%d unum:%d", 
        request.State.WorldModel.Cycle, 
        len(request.Pairs), 
        request.RegisterResponse.UniformNumber),request.RegisterResponse.UniformNumber)

    var bestIndex int32 = -1
    maxEvaluation := math.Inf(-1) 

    for i := int32(0) ; i < int32(len(request.Pairs)) ; i++ {
        pair := request.Pairs[i]
		if pair == nil {
			continue
		}
        if pair.Action.TargetPoint.X > float32(maxEvaluation) {
            maxEvaluation = float64(pair.Action.TargetPoint.X)
            bestIndex = i
        }
    }

    g.debugLog(fmt.Sprintf("Best action selected: %v", request.Pairs[bestIndex]),request.RegisterResponse.UniformNumber)

    return &pb.BestPlannerActionResponse{Index: bestIndex}, nil
}



func serve(port string, sharedLock *sync.Mutex, sharedNumberOfConnections *int32) {

	grpcServer := grpc.NewServer()
	gameService := &GameHandler{
		sharedLock:               sharedLock,
		sharedNumberOfConnections: sharedNumberOfConnections,
		agents:                   make(map[int32]*GrpcAgent),
		playerTypes:              make(map[int32]*pb.PlayerType),
	}
    fmt.Println("Server started")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		fmt.Printf("failed to listen: %v", err)
	}
    fmt.Println("Server listening")
	pb.RegisterGameServer(grpcServer, gameService)
	fmt.Printf("Starting server on port %s...\n", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func main() {
	port := "50051"
    sharedLock := &sync.Mutex{}
    sharedNumberOfConnections := new(int32)   

	serve(port, sharedLock, sharedNumberOfConnections)
}
