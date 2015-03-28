@0xea9bd94cc6ca8c10;
using Go = import "go.capnp";
$Go.package("main");
$Go.import("testpkg");


struct PbodyCapn { 
   isRequest  @0:   Bool; 
   serialnum  @1:   Int64; 
   paysize    @2:   Int64; 
   abTm       @3:   Int64; 
   lpTm       @4:   Int64; 
   paymac     @5:   List(UInt8); 
   payload    @6:   List(UInt8); 
} 

struct PelicanPacketCapn { 
   isRequest  @0:   Bool; 
   key        @1:   Text; 
   body       @2:   List(PbodyCapn); 
} 

##compile with:

##
##
##   capnp compile -ogo odir/schema.capnp

